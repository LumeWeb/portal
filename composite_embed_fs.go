package portal

import (
	"bytes"
	"fmt"
	"github.com/samber/lo"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type compositeEmbedFS struct {
	mounts map[string]fs.FS
	mu     sync.RWMutex
}

// flatFS implements fs.FS for the flattened view

type flatFS struct {
	files map[string][]byte // filename -> content
}

type flatDirEntry struct {
	name string
	isDir bool
}

func (e *flatDirEntry) Name() string {
	return e.name
}

func (e *flatDirEntry) IsDir() bool {
	return e.isDir
}

func (e *flatDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}

func (e *flatDirEntry) Info() (fs.FileInfo, error) {
	if e.isDir {
		return &flatDirInfo{name: e.name}, nil
	}
	return &flatFileInfo{name: e.name, size: 0}, nil
}

// flatFile implements fs.File for files in flatFS
type flatFile struct {
	*bytes.Reader
	name string
}

func (f *flatFile) Stat() (fs.FileInfo, error) {
	return &flatFileInfo{
		name: f.name,
		size: int64(f.Len()),
	}, nil
}

func (f *flatFile) Close() error {
	return nil
}

type flatFileInfo struct {
	name string
	size int64
}

func (f *flatFileInfo) Name() string       { return f.name }
func (f *flatFileInfo) Size() int64        { return f.size }
func (f *flatFileInfo) Mode() fs.FileMode  { return 0444 }
func (f *flatFileInfo) ModTime() time.Time { return time.Time{} }
func (f *flatFileInfo) IsDir() bool        { return false }
func (f *flatFileInfo) Sys() interface{}   { return nil }

// NewCompositeFS creates a new compositeEmbedFS
func newCompositeFS() *compositeEmbedFS {
	return &compositeEmbedFS{
		mounts: make(map[string]fs.FS),
	}
}

// Mount registers a filesystem under the given path prefix
func (c *compositeEmbedFS) Mount(prefix string, filesystem fs.FS) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Normalize path
	prefix = path.Clean(prefix)
	if prefix == "." {
		prefix = ""
	} else if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	c.mounts[prefix] = filesystem
}

func (c *compositeEmbedFS) Mounts() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return lo.Keys(c.mounts)
}

// findFS finds the appropriate filesystem and adjusts the path
func (c *compositeEmbedFS) findFS(name string) (fs.FS, string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Clean and normalize path
	name = path.Clean(name)
	if name == "." {
		name = ""
	}

	// Try exact match first
	if name == "" {
		for prefix, fsys := range c.mounts {
			if prefix == "" {
				return fsys, ".", nil
			}
		}
	}

	// Find the best matching prefix
	var bestMatch string
	var bestFS fs.FS

	normalizedName := strings.TrimSuffix(name, "/") + "/"

	for prefix, fsys := range c.mounts {
		if strings.HasPrefix(normalizedName, prefix) {
			// Empty prefix is a special case that should match everything
			if prefix == "" {
				bestMatch = prefix
				bestFS = fsys
				continue
			}
			// Otherwise prefer the longest matching prefix
			if len(prefix) > len(bestMatch) {
				bestMatch = prefix
				bestFS = fsys
			}
		}
	}

	if bestFS == nil {
		return nil, "", &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	// Calculate relative path within the filesystem
	relPath := name
	if bestMatch != "" {
		relPath = strings.TrimPrefix(normalizedName, bestMatch)
		if relPath == "" {
			relPath = "."
		}
		relPath = strings.TrimPrefix(relPath, "/")
		relPath = strings.TrimSuffix(relPath, "/")
	}

	return bestFS, relPath, nil
}

// Open implements fs.FS
func (c *compositeEmbedFS) Open(name string) (fs.File, error) {
	fsys, relPath, err := c.findFS(name)
	if err != nil {
		return nil, err
	}

	return fsys.Open(relPath)
}

// ReadDir reads the named directory and returns a list of directory entries
func (c *compositeEmbedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fsys, relPath, err := c.findFS(name)
	if err != nil {
		return nil, err
	}

	return fs.ReadDir(fsys, relPath)
}

// ReadFile reads the named file and returns its contents
func (c *compositeEmbedFS) ReadFile(name string) ([]byte, error) {
	fsys, relPath, err := c.findFS(name)
	if err != nil {
		return nil, err
	}

	return fs.ReadFile(fsys, relPath)
}

// Sub returns an FS corresponding to the subtree rooted at dir
func (c *compositeEmbedFS) Sub(dir string) (fs.FS, error) {
	// Create a new CompositeFS for the subdirectory
	result := newCompositeFS()

	c.mu.RLock()
	defer c.mu.RUnlock()

	dir = path.Clean(dir)
	if dir == "." {
		dir = ""
	} else if !strings.HasSuffix(dir, "/") {
		dir = dir + "/"
	}

	// Mount relevant filesystems to the new CompositeFS
	for prefix, filesystem := range c.mounts {
		if strings.HasPrefix(dir, prefix) {
			// This filesystem should be included in the sub FS
			newPrefix := strings.TrimPrefix(dir, prefix)
			newPrefix = strings.TrimSuffix(newPrefix, "/")

			if subFSInterface, ok := filesystem.(fs.SubFS); ok {
				subFilesystem, err := subFSInterface.Sub(newPrefix)
				if err != nil {
					// If the underlying FS doesn't have the sub path, skip it
					continue
				}
				result.Mount("", subFilesystem)
			} else {
				// For non-SubFS filesystems, use the standard library's Sub
				subFilesystem, err := fs.Sub(filesystem, newPrefix)
				if err != nil {
					continue
				}
				result.Mount("", subFilesystem)
			}
		}
	}

	return result, nil
}

// Flatten creates a single-level filesystem containing all files from mounted filesystems.
// Returns an error if any filename conflicts are detected (same filename exists in multiple mounted filesystems).
func (c *compositeEmbedFS) Flatten() (fs.FS, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	flat := &flatFS{
		files: make(map[string][]byte),
	}

	for _, fsys := range c.mounts {
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			// Read file contents
			file, err := fsys.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open %s: %w", path, err)
			}
			defer file.Close()

			content, err := io.ReadAll(file)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", path, err)
			}

			// Use just the filename (last component of path)
			filename := filepath.Base(path)
			if _, exists := flat.files[filename]; exists {
				return fmt.Errorf("filename conflict: %q exists in multiple mounted filesystems", filename)
			}
			flat.files[filename] = content
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("error walking filesystem: %w", err)
		}
	}

	return flat, nil
}

func (f *flatFS) Stat(name string) (fs.FileInfo, error) {
	// Handle root directory case
	if name == "." {
		return &flatDirInfo{
			name: ".",
		}, nil
	}

	// Only allow simple filenames in root directory
	filename := filepath.Base(name)
	if filename != name {
		return nil, &fs.PathError{
			Op:   "stat",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}

	// Check if it's a file
	if content, ok := f.files[filename]; ok {
		return &flatFileInfo{
			name: filename,
			size: int64(len(content)),
		}, nil
	}

	// Not found
	return nil, &fs.PathError{
		Op:   "stat",
		Path: name,
		Err:  fs.ErrNotExist,
	}
}

type flatDirInfo struct {
	name string
}

func (f *flatDirInfo) Name() string       { return f.name }
func (f *flatDirInfo) Size() int64        { return 0 }
func (f *flatDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0555 }
func (f *flatDirInfo) ModTime() time.Time { return time.Time{} }
func (f *flatDirInfo) IsDir() bool        { return true }
func (f *flatDirInfo) Sys() interface{}   { return nil }

func (f *flatFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}

	entries := make([]fs.DirEntry, 0, len(f.files)+1)
	
	// Add root directory entry
	entries = append(entries, &flatDirEntry{
		name: ".",
		isDir: true,
	})

	// Add all files
	for filename := range f.files {
		entries = append(entries, &flatDirEntry{
			name: filename,
			isDir: false,
		})
	}

	return entries, nil
}

func (f *flatFS) Open(name string) (fs.File, error) {
	// Only allow simple filenames in root directory
	filename := filepath.Base(name)
	if filename != name {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}

	content, ok := f.files[filename]
	if !ok {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}

	return &flatFile{
		Reader: bytes.NewReader(content),
		name:   filename,
	}, nil
}
