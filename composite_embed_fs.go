package portal

import (
	"github.com/samber/lo"
	"io/fs"
	"path"
	"strings"
	"sync"
)

type compositeEmbedFS struct {
	mounts map[string]fs.FS
	mu     sync.RWMutex
}

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

	// Find the longest matching prefix
	var bestMatch string
	var bestFS fs.FS

	// Add trailing slash for matching
	namePath := name + "/"

	for prefix, fsys := range c.mounts {
		if strings.HasPrefix(namePath, prefix) && len(prefix) > len(bestMatch) {
			bestMatch = prefix
			bestFS = fsys
		}
	}

	if bestFS == nil {
		return nil, "", &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	// Calculate relative path within the filesystem
	relPath := name
	if bestMatch != "" {
		relPath = strings.TrimPrefix(name, bestMatch[:len(bestMatch)-1])
		if relPath == "" {
			relPath = "."
		}
		if strings.HasPrefix(relPath, "/") {
			relPath = relPath[1:]
		}
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
		if strings.HasPrefix(prefix, dir) {
			// This filesystem should be included in the sub FS
			newPrefix := strings.TrimPrefix(prefix, dir)
			if subFSInterface, ok := filesystem.(interface{ Sub(string) (fs.FS, error) }); ok && strings.HasPrefix(dir, prefix) {
				// The directory is within this filesystem
				relDir := strings.TrimPrefix(dir, prefix)
				if subFilesystem, err := subFSInterface.Sub(relDir); err == nil {
					result.Mount(newPrefix, subFilesystem)
				}
			} else {
				// Just include the whole filesystem with adjusted prefix
				result.Mount(newPrefix, filesystem)
			}
		}
	}

	return result, nil
}
