package s5

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"git.lumeweb.com/LumeWeb/portal/protocols/s5"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	"git.lumeweb.com/LumeWeb/portal/storage"

	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
)

var _ io.ReadSeekCloser = (*S5File)(nil)

type S5File struct {
	reader   io.ReadCloser
	hash     []byte
	storage  storage.StorageService
	metadata metadata.MetadataService
	record   *metadata.UploadMetadata
	protocol *s5.S5Protocol
	cid      *encoding.CID
	read     bool
	tus      *s5.TusHandler
}

type FileParams struct {
	Storage  storage.StorageService
	Metadata metadata.MetadataService
	Hash     []byte
	Protocol *s5.S5Protocol
	Tus      *s5.TusHandler
}

func NewFile(params FileParams) *S5File {
	return &S5File{
		storage:  params.Storage,
		metadata: params.Metadata,
		hash:     params.Hash,
		protocol: params.Protocol,
		tus:      params.Tus,
	}
}

func (f *S5File) Exists() bool {
	_, err := f.metadata.GetUpload(context.Background(), f.hash)

	if err != nil {
		return false
	}

	return true
}

func (f *S5File) Read(p []byte) (n int, err error) {
	err = f.init(0)
	if err != nil {
		return 0, err
	}
	f.read = true

	return f.reader.Read(p)
}

func (f *S5File) Seek(offset int64, whence int) (int64, error) {
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

func (f *S5File) Close() error {
	if f.reader != nil {
		r := f.reader
		f.reader = nil
		return r.Close()
	}

	return nil
}

func (f *S5File) init(offset int64) error {
	if f.reader == nil {

		reader, err := f.tus.GetUploadReader(f.hash, offset)

		if err == nil {
			f.reader = reader
			f.read = false
			return nil
		}

		reader, err = f.storage.DownloadObject(context.Background(), f.StorageProtocol(), f.hash, offset)
		if err != nil {
			return err
		}

		f.reader = reader
		f.read = false
	}

	return nil
}

func (f *S5File) Record() (*metadata.UploadMetadata, error) {
	if f.record == nil {
		record, err := f.metadata.GetUpload(context.Background(), f.Hash())

		if err != nil {
			return nil, errors.New("file does not exist")
		}

		f.record = &record
	}

	return f.record, nil
}

func (f *S5File) Hash() []byte {
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

func (f *S5File) HashString() string {
	record, err := f.Record()
	if err != nil {
		return ""
	}

	return hex.EncodeToString(record.Hash)
}

func (f *S5File) Name() string {
	cid, _ := f.CID().ToString()

	return cid
}

func (f *S5File) Modtime() time.Time {
	record, err := f.Record()
	if err != nil {
		return time.Unix(0, 0)
	}

	return record.Created
}
func (f *S5File) Size() uint64 {
	record, err := f.Record()
	if err != nil {
		return 0
	}

	return record.Size
}
func (f *S5File) CID() *encoding.CID {
	if f.cid == nil {
		multihash := encoding.MultihashFromBytes(f.Hash(), types.HashTypeBlake3)
		cid := encoding.NewCID(types.CIDTypeRaw, *multihash, f.Size())
		f.cid = cid
	}
	return f.cid
}

func (f *S5File) Mime() string {
	record, err := f.Record()
	if err != nil {
		return ""
	}

	return record.MimeType
}

func (f *S5File) StorageProtocol() storage.StorageProtocol {
	return s5.GetStorageProtocol(f.protocol)
}

func (f *S5File) Proof() ([]byte, error) {
	object, err := f.storage.DownloadObjectProof(context.Background(), f.StorageProtocol(), f.hash)

	if err != nil {
		return nil, err
	}

	proof, err := io.ReadAll(object)
	if err != nil {
		return nil, err
	}

	err = object.Close()
	if err != nil {
		return nil, err
	}

	return proof, nil
}
