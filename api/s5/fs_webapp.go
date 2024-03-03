package s5

import (
	"errors"
	"io/fs"

	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/metadata"
)

var _ fs.FS = (*webAppFs)(nil)

type webAppFs struct {
	root *encoding.CID
	s5   *S5API
}

func (w webAppFs) Open(name string) (fs.File, error) {
	file := w.s5.newFile(FileParams{
		Hash: w.root.Hash.HashBytes(),
		Type: w.root.Type,
	})

	manifest, err := file.Manifest()
	if err != nil {
		return nil, err
	}

	webApp, ok := manifest.(*metadata.WebAppMetadata)

	if !ok {
		return nil, errors.New("manifest is not a web app")
	}

	item, ok := webApp.Paths.Get(name)

	if !ok {
		return nil, fs.ErrNotExist
	}
	return w.s5.newFile(FileParams{
		Hash: item.Cid.Hash.HashBytes(),
		Type: item.Cid.Type,
		Name: name,
	}), nil
}

func newWebAppFs(root *encoding.CID, s5 *S5API) *webAppFs {
	return &webAppFs{
		root: root,
		s5:   s5,
	}
}
