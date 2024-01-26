package interfaces

import (
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"io"
	"time"
)

type File interface {
	Record() (*models.Upload, error)
	Hash() []byte
	HashString() string
	Name() string
	Modtime() time.Time
	Mime() string
	Size() uint64
	CID() *encoding.CID
	Exists() bool
	io.ReadSeekCloser
}
