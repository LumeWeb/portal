package storage

import (
	"encoding/hex"
	"errors"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
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
	cid     *encoding.CID
	read    bool
}

func NewFile(hash []byte, storage interfaces.StorageService) *File {
	return &File{hash: hash, storage: storage, read: false}
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
	f.read = true

	return f.reader.Read(p)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if !f.read {
			return 0, nil
		}

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
	case io.SeekCurrent:
		return 0, errors.New("not supported")
	case io.SeekEnd:
		return int64(f.Size()), nil
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
		f.read = false
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

func (f *File) Name() string {
	cid, _ := f.CID().ToString()

	return cid
}

func (f *File) Modtime() time.Time {
	record, err := f.Record()
	if err != nil {
		return time.Unix(0, 0)
	}

	return record.CreatedAt
}
func (f *File) Size() uint64 {
	record, err := f.Record()
	if err != nil {
		return 0
	}

	return record.Size
}
func (f *File) CID() *encoding.CID {
	if f.cid == nil {
		multihash := encoding.MultihashFromBytes(f.Hash(), types.HashTypeBlake3)
		cid := encoding.NewCID(types.CIDTypeRaw, *multihash, f.Size())
		f.cid = cid
	}
	return f.cid
}
