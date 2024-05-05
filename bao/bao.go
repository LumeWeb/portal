package bao

import (
	"bytes"
	_ "embed"
	"errors"
	"io"
	"time"

	"github.com/docker/go-units"

	"github.com/samber/lo"

	"go.uber.org/zap"

	"lukechampine.com/blake3/bao"
)

var _ io.ReadCloser = (*Verifier)(nil)
var _ io.WriterAt = (*proofWriter)(nil)

var ErrVerifyFailed = errors.New("verification failed")

const groupLog = 8
const groupChunks = (1 << groupLog) * units.KiB

type Verifier struct {
	r          io.ReadCloser
	proof      Result
	read       uint64
	buffer     *bytes.Buffer
	logger     *zap.Logger
	readTime   []time.Duration
	verifyTime time.Duration
}

type Result struct {
	Hash   []byte
	Proof  []byte
	Length uint
}

func (v *Verifier) Read(p []byte) (int, error) {
	// Initial attempt to read from the buffer
	n, err := v.buffer.Read(p)
	if n == len(p) {
		// If the buffer already had enough data to fulfill the request, return immediately
		return n, nil
	} else if err != nil && err != io.EOF {
		// For errors other than EOF, return the error immediately
		return n, err
	}

	buf := make([]byte, groupChunks)
	// Continue reading from the source and verifying until we have enough data or hit an error
	for v.buffer.Len() < len(p)-n {
		readStart := time.Now()
		bytesRead, err := io.ReadFull(v.r, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return n, err // Return any read error immediately
		}

		readEnd := time.Now()

		v.readTime = append(v.readTime, readEnd.Sub(readStart))

		timeStart := time.Now()

		if bytesRead > 0 {
			if status := bao.VerifyChunk(buf[:bytesRead], v.proof.Proof, groupLog, v.read, [32]byte(v.proof.Hash)); !status {
				return n, errors.Join(ErrVerifyFailed, err)
			}
			v.read += uint64(bytesRead)
			v.buffer.Write(buf[:bytesRead]) // Append new data to the buffer
		}

		timeEnd := time.Now()
		v.verifyTime += timeEnd.Sub(timeStart)

		if err == io.EOF {
			// If EOF, break the loop as no more data can be read
			break
		}
	}

	if len(v.readTime) > 0 {
		averageReadTime := lo.Reduce(v.readTime, func(acc time.Duration, cur time.Duration, _ int) time.Duration {
			return acc + cur
		}, time.Duration(0)) / time.Duration(len(v.readTime))

		v.logger.Debug("Read time", zap.Duration("average", averageReadTime))
	}

	averageVerifyTime := v.verifyTime / time.Duration(v.read/groupChunks)
	v.logger.Debug("Verification time", zap.Duration("average", averageVerifyTime))

	// Attempt to read the remainder of the data from the buffer
	additionalBytes, _ := v.buffer.Read(p[n:])
	n += additionalBytes

	if v.buffer.Len() == 0 && err == io.EOF {
		// If the buffer is empty and the underlying reader reached EOF, return EOF
		return n, io.EOF
	}

	return n, nil
}

func (v *Verifier) Close() error {
	return v.r.Close()
}
func Hash(r io.Reader, size uint64) (*Result, error) {
	reader := newSizeReader(r)
	writer := newProofWriter(int(size))

	hash, err := bao.Encode(writer, reader, int64(size), groupLog, true)
	if err != nil {
		return nil, err
	}

	return &Result{
		Hash:   hash[:],
		Proof:  writer.buf,
		Length: uint(size),
	}, nil
}

func NewVerifier(r io.ReadCloser, proof Result, logger *zap.Logger) *Verifier {
	return &Verifier{
		r:      r,
		proof:  proof,
		buffer: new(bytes.Buffer),
		logger: logger,
	}
}

type proofWriter struct {
	buf []byte
}

func (p proofWriter) WriteAt(b []byte, off int64) (n int, err error) {
	if copy(p.buf[off:], b) != len(b) {
		panic("bad buffer size")
	}
	return len(b), nil
}

func newProofWriter(size int) *proofWriter {
	return &proofWriter{
		buf: make([]byte, bao.EncodedSize(size, groupLog, true)),
	}
}

type sizeReader struct {
	reader io.Reader
	read   int64
}

func (s *sizeReader) Read(p []byte) (int, error) {
	n, err := s.reader.Read(p)
	s.read += int64(n)
	return n, err
}

func newSizeReader(r io.Reader) *sizeReader {
	return &sizeReader{
		reader: r,
		read:   0,
	}
}
