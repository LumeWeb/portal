package storage

import (
	"encoding/hex"
	"errors"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"io"
	"time"
)

var (
	_ io.ReadSeekCloser = (*File)(nil)
)

type File struct {
	reader  io.ReadCloser
	hash    []byte
	storage interfaces.StorageService
	record  *models.Upload
}

func NewFile(hash []byte, storage interfaces.StorageService) *File {
	return &File{hash: hash, storage: storage}
}

func (f *File) Exists() bool {
	exists, _ := f.storage.FileExists(f.hash)

	return exists
}

func (f *File) Read(p []byte) (n int, err error) {
	err = f.init(0)
	if err != nil {
		return 0, err
	}

	return f.reader.Read(p)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if f.reader != nil {
			err := f.reader.Close()
			if err != nil {
				return 0, err
			}
			f.reader = nil
		}
		err := f.init(offset)
		if err != nil {
			return 0, err
		}
	case io.SeekCurrent, io.SeekEnd:
		return 0, errors.New("not supported")
	default:
		return 0, errors.New("invalid whence")
	}

	return 0, nil
}

func (f *File) Close() error {
	if f.reader != nil {
		r := f.reader
		f.reader = nil
		return r.Close()
	}

	return nil
}

func (f *File) init(offset int64) error {
	if f.reader == nil {
		reader, _, err := f.storage.GetFile(f.hash, offset)
		if err != nil {
			return err
		}

		f.reader = reader
	}

	return nil
}

func (f *File) Record() (*models.Upload, error) {
	if f.record == nil {
		exists, record := f.storage.FileExists(f.hash)

		if !exists {
			return nil, errors.New("file does not exist")
		}

		f.record = &record
	}

	return f.record, nil
}

func (f *File) Hash() []byte {
	hashStr := f.HashString()

	if hashStr == "" {
		return nil
	}

	str, err := hex.DecodeString(hashStr)
	if err != nil {
		return nil
	}

	return str
}

func (f *File) HashString() string {
	record, err := f.Record()
	if err != nil {
		return ""
	}

	return record.Hash
}

func (f *File) Name() time.Time {
	record, err := f.Record()
	if err != nil {
		return time.Unix(0, 0)
	}

	return record.CreatedAt
}

func (f *File) Modtime() time.Time {
	record, err := f.Record()
	if err != nil {
		return time.Unix(0, 0)
	}

	return record.CreatedAt
}