package http

import (
	"errors"
	"fmt"
	"go.lumeweb.com/portal/core"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"time"
)

type BundleFileSystem struct {
	bundle *core.WebBundle
	prefix string
}

// BundleFile implements http.File for a file in the bundle
type BundleFile struct {
	name    string
	content []byte
	offset  int64
}

func NewBundleFileSystem(bundle *core.WebBundle, prefix string) *BundleFileSystem {
	return &BundleFileSystem{
		bundle: bundle,
		prefix: prefix,
	}
}

// Open implements http.FileSystem
func (fs *BundleFileSystem) Open(name string) (http.File, error) {
	// Clean the path and join with prefix
	name = path.Clean("/" + name)
	fullPath := path.Join(fs.prefix, name)

	file, err := fs.bundle.Files.Open(fullPath)

	if err != nil {
		return nil, os.ErrNotExist
	}
	content, err := io.ReadAll(file)

	if err != nil {
		return nil, os.ErrNotExist
	}

	return &BundleFile{
		name:    name,
		content: content,
		offset:  0,
	}, nil
}

// Close implements http.File
func (f *BundleFile) Close() error {
	return nil
}

// Read implements http.File
func (f *BundleFile) Read(p []byte) (n int, err error) {
	if f.offset >= int64(len(f.content)) {
		return 0, io.EOF
	}
	n = copy(p, f.content[f.offset:])
	f.offset += int64(n)
	return
}

// Seek implements http.File
func (f *BundleFile) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = f.offset + offset
	case io.SeekEnd:
		abs = int64(len(f.content)) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative offset")
	}
	f.offset = abs
	return abs, nil
}

// Stat implements http.File
func (f *BundleFile) Stat() (os.FileInfo, error) {
	return &bundleFileInfo{
		name:    path.Base(f.name),
		size:    int64(len(f.content)),
		mode:    0444, // read-only
		modTime: time.Now(),
	}, nil
}

// Readdir implements http.File
func (f *BundleFile) Readdir(count int) ([]fs.FileInfo, error) {
	return nil, fmt.Errorf("not a directory")
}

// bundleFileInfo implements os.FileInfo
type bundleFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (fi *bundleFileInfo) Name() string       { return fi.name }
func (fi *bundleFileInfo) Size() int64        { return fi.size }
func (fi *bundleFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *bundleFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *bundleFileInfo) IsDir() bool        { return false }
func (fi *bundleFileInfo) Sys() interface{}   { return nil }
