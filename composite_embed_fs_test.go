package portal

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompositeFS_BasicOperations(t *testing.T) {
	t.Parallel()

	// Create test filesystems
	fs1 := fstest.MapFS{
		"file1.txt":     &fstest.MapFile{Data: []byte("file1 content")},
		"sub/file2.txt": &fstest.MapFile{Data: []byte("file2 content")},
	}
	fs2 := fstest.MapFS{
		"file3.txt":     &fstest.MapFile{Data: []byte("file3 content")},
		"sub/file4.txt": &fstest.MapFile{Data: []byte("file4 content")},
	}

	// Create composite FS
	cfs := newCompositeFS()
	cfs.Mount("fs1", fs1)
	cfs.Mount("fs2", fs2)

	t.Run("Open files from different mounts", func(t *testing.T) {
		tests := []struct {
			path    string
			content string
		}{
			{"fs1/file1.txt", "file1 content"},
			{"fs1/sub/file2.txt", "file2 content"},
			{"fs2/file3.txt", "file3 content"},
			{"fs2/sub/file4.txt", "file4 content"},
		}

		for _, tt := range tests {
			file, err := cfs.Open(tt.path)
			require.NoError(t, err, "Open(%q)", tt.path)

			stat, err := file.Stat()
			require.NoError(t, err)
			assert.False(t, stat.IsDir())

			data := make([]byte, stat.Size())
			_, err = file.Read(data)
			require.NoError(t, err)
			assert.Equal(t, tt.content, string(data))
		}
	})

	t.Run("Nonexistent files", func(t *testing.T) {
		_, err := cfs.Open("fs1/nonexistent.txt")
		assert.ErrorIs(t, err, fs.ErrNotExist)

		_, err = cfs.Open("nonexistent/file.txt")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("ReadDir", func(t *testing.T) {
		entries, err := cfs.ReadDir("fs1")
		require.NoError(t, err)
		assert.Len(t, entries, 2) // file1.txt and sub/
	})

	t.Run("Sub", func(t *testing.T) {
		subFS, err := cfs.Sub("fs1/sub")
		require.NoError(t, err)

		file, err := subFS.Open("file2.txt")
		require.NoError(t, err)
		defer func(file fs.File) {
			err = file.Close()
			if err != nil {
				require.NoError(t, err)
			}
		}(file)

		data, err := fs.ReadFile(subFS, "file2.txt")
		require.NoError(t, err)
		assert.Equal(t, "file2 content", string(data))

		// Test nested sub
		// Test nested sub using fs.Sub
		nestedSubFS, err := fs.Sub(subFS, ".")
		require.NoError(t, err)

		nestedFile, err := nestedSubFS.Open("file2.txt")
		require.NoError(t, err)
		defer func(nestedFile fs.File) {
			err = nestedFile.Close()
			if err != nil {
				require.NoError(t, err)
			}
		}(nestedFile)

		nestedData, err := fs.ReadFile(nestedSubFS, "file2.txt")
		require.NoError(t, err)
		assert.Equal(t, "file2 content", string(nestedData))
	})
}

func TestFlatten(t *testing.T) {
	t.Parallel()

	// Create test filesystems with unique filenames
	fs1 := fstest.MapFS{
		"file1.txt": &fstest.MapFile{Data: []byte("file1 content")},
		"file2.txt": &fstest.MapFile{Data: []byte("file2 content")},
	}
	fs2 := fstest.MapFS{
		"file3.txt": &fstest.MapFile{Data: []byte("file3 content")},
		"file4.txt": &fstest.MapFile{Data: []byte("file4 content")},
	}

	// Create composite FS
	cfs := newCompositeFS()
	cfs.Mount("prefix1", fs1)
	cfs.Mount("prefix2", fs2)

	// Flatten it
	flatFS, err := cfs.Flatten()
	require.NoError(t, err)

	t.Run("All files exist in root directory", func(t *testing.T) {
		tests := []struct {
			path    string
			content string
		}{
			{"file1.txt", "file1 content"},
			{"file2.txt", "file2 content"},
			{"file3.txt", "file3 content"},
			{"file4.txt", "file4 content"},
		}

		for _, tt := range tests {
			data, err := fs.ReadFile(flatFS, tt.path)
			require.NoError(t, err, "ReadFile(%q)", tt.path)
			assert.Equal(t, tt.content, string(data))
		}
	})

	t.Run("No directory structure remains", func(t *testing.T) {
		// Verify no subdirectories exist
		_, err := flatFS.Open("prefix1")
		assert.ErrorIs(t, err, fs.ErrNotExist)

		_, err = flatFS.Open("prefix2")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("Nonexistent files", func(t *testing.T) {
		_, err := flatFS.Open("nonexistent.txt")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestFlatten_Empty(t *testing.T) {
	t.Parallel()

	cfs := newCompositeFS()
	flatFS, err := cfs.Flatten()
	require.NoError(t, err)

	// Should be empty but not error
	_, err = flatFS.Open("anyfile.txt")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestFlatten_RealFiles(t *testing.T) {
	t.Parallel()

	// Create temp dirs with real files
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file1.txt"), []byte("file1 content"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(dir1, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "sub", "file2.txt"), []byte("file2 content"), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(dir2, "file3.txt"), []byte("file3 content"), 0644))

	// Create composite FS with OS filesystems
	cfs := newCompositeFS()
	cfs.Mount("real1", os.DirFS(dir1))
	cfs.Mount("real2", os.DirFS(dir2))

	// Flatten it
	flatFS, err := cfs.Flatten()
	require.NoError(t, err)

	// Verify files are flattened to root directory
	data, err := fs.ReadFile(flatFS, "file1.txt")
	require.NoError(t, err)
	assert.Equal(t, "file1 content", string(data))

	data, err = fs.ReadFile(flatFS, "file2.txt")
	require.NoError(t, err)
	assert.Equal(t, "file2 content", string(data))

	data, err = fs.ReadFile(flatFS, "file3.txt")
	require.NoError(t, err)
	assert.Equal(t, "file3 content", string(data))

	// Verify directory structure is gone
	_, err = flatFS.Open("real1")
	assert.ErrorIs(t, err, fs.ErrNotExist)

	_, err = flatFS.Open("sub")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}
