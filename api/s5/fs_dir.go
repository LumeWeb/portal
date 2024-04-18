package s5

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/LumeWeb/libs5-go/node"

	"github.com/LumeWeb/libs5-go/encoding"
	"github.com/LumeWeb/libs5-go/metadata"
)

var _ fs.FS = (*dirFs)(nil)

type dirFs struct {
	root *encoding.CID
	s5   *S5API
}

func (w *dirFs) Open(name string) (fs.File, error) {
	file := w.s5.newFile(FileParams{
		Hash: w.root.Hash.HashBytes(),
		Type: w.root.Type,
	})

	manifest, err := file.Manifest()
	if err != nil {
		return nil, err
	}

	dir, ok := manifest.(*metadata.DirectoryMetadata)

	if !ok {
		return nil, errors.New("manifest is not a directory")
	}

	segments := strings.Split(name, "/")

	if len(segments) == 1 {
		return w.openDirectly(name, dir)
	}

	nextDirName := segments[0]
	remainingPath := strings.Join(segments[1:], "/")

	return w.openNestedDir(nextDirName, remainingPath, dir)
}

func (w *dirFs) openDirectly(name string, dir *metadata.DirectoryMetadata) (fs.File, error) {
	file := dir.Files.Get(name)
	subDir := dir.Directories.Get(name)

	if file != nil {
		return w.s5.newFile(FileParams{
			Hash: file.File.CID().Hash.HashBytes(),
			Type: file.File.CID().Type,
			Name: file.Name,
		}), nil
	}

	if subDir != nil {
		cid, err := w.resolveDirCid(subDir)
		if err != nil {
			return nil, err
		}

		return w.s5.newFile(FileParams{
			Hash: cid.Hash.HashBytes(),
			Type: cid.Type,
			Name: name,
		}), nil
	}

	if name == "." {
		return w.s5.newFile(FileParams{
			Hash: w.root.Hash.HashBytes(),
			Type: w.root.Type,
			Name: name,
		}), nil

	}

	return nil, fs.ErrNotExist
}

func (w dirFs) openNestedDir(name string, remainingPath string, dir *metadata.DirectoryMetadata) (fs.File, error) {
	subDir := dir.Directories.Get(name)

	if subDir == nil {
		return nil, fs.ErrNotExist
	}

	cid, err := w.resolveDirCid(subDir)
	if err != nil {
		return nil, err
	}

	nestedFs := newDirFs(cid, w.s5)

	return nestedFs.Open(remainingPath)

}

func (w *dirFs) resolveDirCid(dir *metadata.DirectoryReference) (*encoding.CID, error) {
	return resolveDirCid(dir, w.s5.getNode())
}

func newDirFs(root *encoding.CID, s5 *S5API) *dirFs {
	return &dirFs{
		root: root,
		s5:   s5,
	}
}
func resolveDirCid(dir *metadata.DirectoryReference, node *node.Node) (*encoding.CID, error) {
	if len(dir.PublicKey) == 0 {
		return nil, errors.New("missing public key")
	}

	entry, err := node.Services().Registry().Get(dir.PublicKey)
	if err != nil {
		return nil, err
	}

	cid, err := encoding.CIDFromRegistry(entry.Data())

	if err != nil {
		return nil, err
	}

	return cid, nil
}
