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
	"path/filepath"
	"strings"
)

type BundleFileSystem struct {
	bundle *core.WebBundle
	prefix string
}

// BundleFile implements http.File for a file in the bundle
type BundleFile struct {
	name   string
	file   fs.File
	seeker io.Seeker
	size   int64
	offset int64
}

func NewBundleFileSystem(bundle *core.WebBundle, prefix string) *BundleFileSystem {
	return &BundleFileSystem{
		bundle: bundle,
		prefix: prefix,
	}
}

// Open implements http.FileSystem
func (fs *BundleFileSystem) Open(name string) (http.File, error) {
	// Normalize path separators and clean the path
	name = filepath.Clean("/" + filepath.ToSlash(name))

	// Reject paths containing directory traversal
	if strings.Contains(name, "../") || name == ".." {
		return nil, os.ErrNotExist
	}

	// Join with prefix
	fullPath := path.Join(fs.prefix, name)

	file, err := fs.bundle.Files.Open(fullPath)
	if err != nil {
		return nil, os.ErrNotExist
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, os.ErrNotExist
	}

	return &BundleFile{
		name:   name,
		file:   file,
		seeker: file.(io.Seeker),
		size:   info.Size(),
		offset: 0,
	}, nil
}

// Close implements http.File
func (f *BundleFile) Close() error {
	return f.file.Close()
}

// Read implements http.File
func (f *BundleFile) Read(p []byte) (n int, err error) {
	// Only seek if we're not at the expected position
	if currentPos, err := f.seeker.Seek(0, io.SeekCurrent); err == nil && currentPos != f.offset {
		_, err = f.seeker.Seek(f.offset, io.SeekStart)
		if err != nil {
			return 0, err
		}
	}

	n, err = f.file.Read(p)
	f.offset += int64(n)
	return n, err
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
		abs = f.size + offset
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
	return f.file.Stat()
}

// Readdir implements http.File
func (f *BundleFile) Readdir(count int) ([]fs.FileInfo, error) {
	return nil, fmt.Errorf("not a directory")
}
