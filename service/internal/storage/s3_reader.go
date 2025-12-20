package storage

// Copied from https://github.com/nikolaydubina/aws-s3-reader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// ChunkSizePolicy is something that can tell how much data to fetch in single request for given S3 Object.
// With more advanced policies, Visit methods will be integrated.
type ChunkSizePolicy interface {
	ChunkSize() int
}

// FixedChunkSizePolicy always returns same chunk size.
type FixedChunkSizePolicy struct {
	Size int
}

func (s FixedChunkSizePolicy) ChunkSize() int { return s.Size }

// S3Reader is a reader of given S3 Object.
// It utilizes HTTP Byte Ranges to read chunks of data from S3 Object.
// It uses zero-memory copy from underlying HTTP Body response.
// It uses early HTTP Body termination, if seeks are beyond current HTTP Body.
// It uses adaptive policy for chunk size fetching.
// This is useful for iterating over very large S3 Objects.
// The reader is safe for concurrent access.
type S3Reader struct {
	mu              sync.RWMutex
	ctx             context.Context
	logger          *core.Logger
	s3client        *s3.Client
	bucket          string
	key             string
	offset          int64 // in s3 object
	size            int64 // in s3 object
	lastByte        int64 // in s3 object that we expect to have in current HTTP Body
	chunkSizePolicy ChunkSizePolicy
	r               io.ReadCloser // temporary holder for current reader
	sink            []byte        // where to read bytes discarding data from readers during in-body seek
}

func NewS3Reader(
	ctx context.Context,
	logger *core.Logger,
	s3client *s3.Client,
	bucket string,
	key string,
	chunkSizePolicy ChunkSizePolicy,
) *S3Reader {
	return &S3Reader{
		ctx:             ctx,
		logger:          logger,
		s3client:        s3client,
		bucket:          bucket,
		key:             key,
		chunkSizePolicy: chunkSizePolicy,
	}
}

// Seek assumes always can seek to position in S3 object.
// Seeking beyond S3 file size will result failures in Read calls.
func (s *S3Reader) Seek(offset int64, whence int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.logger.Debug("seeking",
		zap.String("bucket", s.bucket),
		zap.String("key", s.key),
		zap.Int64("offset", offset),
		zap.Int("whence", whence),
		zap.Int64("current_offset", s.offset),
	)

	discardBytes := 0

	switch whence {
	case io.SeekCurrent:
		if offset < 0 {
			// seeking backwards, reset connection to start at new offset
			s.logger.Debug("seeking backwards from current position, resetting connection")
			s.reset()
			s.offset += offset
			discardBytes = 0
		} else {
			discardBytes = int(offset)
			s.offset += offset
		}
	case io.SeekStart:
		// seeking backwards results in dropping current http body.
		// since http body reader can read only forwards.
		if offset < s.offset {
			s.logger.Debug("seeking backwards, resetting connection")
			s.reset()
		}
		oldOffset := s.offset
		s.offset = offset
		discardBytes = int(offset - oldOffset)
		if discardBytes < 0 {
			discardBytes = 0
		}
	case io.SeekEnd:
		if offset > 0 {
			return 0, errors.New("cannot seek beyond end")
		}
		size, err := s.getSize()
		if err != nil {
			return 0, fmt.Errorf("failed to get object size for seeking: %w", err)
		}
		noffset := int64(size) + offset
		discardBytes = int(noffset - s.offset)
		s.offset = noffset
	default:
		return 0, errors.New("unsupported whence")
	}

	if s.offset > s.lastByte {
		s.logger.Debug("seek beyond current range, resetting connection")
		s.reset()
		discardBytes = 0
	}

	if discardBytes > 0 {
		// not seeking
		if s.r == nil {
			// reader is nil, establish it first
			if err := s.fetch(s.chunkSizePolicy.ChunkSize()); err != nil {
				s.logger.Debug("failed to establish reader during seek", zap.Error(err))
				return 0, err
			}
		}
		if discardBytes > len(s.sink) {
			s.sink = make([]byte, discardBytes)
		}
		n, err := s.r.Read(s.sink[:discardBytes])
		if err != nil || n < discardBytes {
			s.logger.Debug("failed to discard bytes during seek, resetting", zap.Error(err))
			s.reset()
		}
	}

	s.logger.Debug("seek completed", zap.Int64("new_offset", s.offset))
	return s.offset, nil
}

func (s *S3Reader) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.logger.Debug("closing S3 reader",
		zap.String("bucket", s.bucket),
		zap.String("key", s.key),
		zap.Int64("final_offset", s.offset),
	)
	if s.r != nil {
		err := s.r.Close()
		if err != nil {
			s.logger.Error("error closing S3 reader", zap.Error(err))
		}
		return err
	}
	return nil
}

func (s *S3Reader) Read(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.r == nil {
		s.logger.Debug("initializing fetch for read",
			zap.String("bucket", s.bucket),
			zap.String("key", s.key),
			zap.Int("buffer_size", len(b)),
			zap.Int("chunk_size", s.chunkSizePolicy.ChunkSize()),
		)
		if err := s.fetch(s.chunkSizePolicy.ChunkSize()); err != nil {
			return 0, err
		}
	}

	n, err := s.r.Read(b)
	s.offset += int64(n)

	if err != nil && errors.Is(err, io.EOF) {
		s.logger.Debug("EOF reached, fetching next chunk")
		// If we read bytes, return them without error
		if n > 0 {
			return n, nil
		}
		// No bytes read, fetch next chunk and return result
		return 0, s.fetch(s.chunkSizePolicy.ChunkSize())
	}

	return n, err
}

func (s *S3Reader) reset() {
	if s.r != nil {
		if err := s.r.Close(); err != nil {
			s.logger.Error("error closing reader during reset", zap.Error(err))
		}
	}
	s.r = nil
	s.lastByte = 0
	s.logger.Debug("reader reset")
}

func (s *S3Reader) getSize() (int, error) {
	// Note: This method should be called with mutex lock held by caller
	if s.size > 0 {
		return int(s.size), nil
	}

	s.logger.Debug("getting object size",
		zap.String("bucket", s.bucket),
		zap.String("key", s.key),
	)

	resp, err := s.s3client.HeadObject(s.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
	})
	if err != nil {
		s.logger.Error("failed to get object size",
			zap.String("bucket", s.bucket),
			zap.String("key", s.key),
			zap.Error(err))
		return 0, err
	}
	s.size = *resp.ContentLength

	s.logger.Debug("object size retrieved",
		zap.String("bucket", s.bucket),
		zap.String("key", s.key),
		zap.Int64("size", s.size))

	return int(s.size), nil
}

func (s *S3Reader) fetch(n int) error {
	s.reset()

	size, err := s.getSize()
	if err != nil {
		return fmt.Errorf("failed to get object size for fetching: %w", err)
	}
	n = min(n, size-int(s.offset))
	if n <= 0 {
		s.logger.Debug("no more data to fetch", zap.Int64("offset", s.offset))
		return io.EOF
	}

	// note, that HTTP Byte Ranges is inclusive range of start-byte and end-byte
	s.lastByte = s.offset + int64(n) - 1

	s.logger.Debug("fetching chunk",
		zap.String("bucket", s.bucket),
		zap.String("key", s.key),
		zap.Int64("offset", s.offset),
		zap.Int64("last_byte", s.lastByte),
		zap.Int("chunk_size", n))

	resp, err := s.s3client.GetObject(s.ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", s.offset, s.lastByte)),
	})
	if err != nil {
		s.logger.Error("failed to fetch chunk",
			zap.String("bucket", s.bucket),
			zap.String("key", s.key),
			zap.Int64("offset", s.offset),
			zap.Int64("last_byte", s.lastByte),
			zap.Error(err))
		return fmt.Errorf("cannot fetch bytes=%d-%d: %w", s.offset, s.lastByte, err)
	}
	s.r = resp.Body
	return nil
}
