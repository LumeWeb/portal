package tus

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tusclient "github.com/eventials/go-tus"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/s3store"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// setupS3 creates a S3 backend with gofakes3 and properly configured client
func setupS3(t *testing.T) (*s3.Client, string) {
	tempDir, err := os.MkdirTemp("", "portal-tus-test-")
	require.NoError(t, err)

	backend, err := s3afero.SingleBucket("test-bucket", afero.NewBasePathFs(afero.NewOsFs(), tempDir), nil)
	require.NoError(t, err)
	faker := gofakes3.New(backend)

	// Launch the gofakes3 server
	httpHandler := faker.Server()
	server := &http.Server{Handler: httpHandler}

	// Find an available port
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: 0})
	require.NoError(t, err)

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	// Start server in goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("gofakes3 server failed: %v", err)
		}
	}()

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	// Cleanup function
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		require.NoError(t, server.Shutdown(ctx))
		cancel()
		require.NoError(t, os.RemoveAll(tempDir))
	})

	// Create S3 client with explicit credentials for gofakes3
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("FAKEKEY", "FAKESECRET", "FAKETOKEN"))),
	)
	require.NoError(t, err)

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})

	return client, "test-bucket"
}

// setupTUSServer creates and starts a TUS server with HTTP endpoint for testing
func setupTUSServer(t *testing.T) (*handler.Handler, *s3store.S3Store, *s3.Client, string, string) {
	client, bucket := setupS3(t)

	// Create S3 store
	store := s3store.New(bucket, client)
	store.ObjectPrefix = "uploads/"

	// Create composer
	composer := handler.NewStoreComposer()
	store.UseIn(composer)

	// Create TUS handler
	tusHandler, err := handler.NewHandler(handler.Config{
		BasePath:                "/files",
		StoreComposer:           composer,
		DisableDownload:         true,
		NotifyCompleteUploads:   false, // Disable to avoid blocking
		NotifyTerminatedUploads: false, // Disable to avoid blocking
		NotifyCreatedUploads:    false, // Disable to avoid blocking
		RespectForwardedHeaders: true,
	})
	require.NoError(t, err)

	// Find an available port for the TUS server
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: 0})
	require.NoError(t, err)

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	// Create HTTP server for TUS handler with proper routing
	server := &http.Server{
		Handler: http.StripPrefix("/files", tusHandler),
	}

	// Start server in goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("TUS server failed: %v", err)
		}
	}()

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	// Cleanup function
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		require.NoError(t, server.Shutdown(ctx))
		cancel()
	})

	return tusHandler, &store, client, bucket, serverURL
}

// uploadWithTusClient uploads data using the go-tus client with chunks to force multipart usage
func uploadWithTusClient(t *testing.T, serverURL string, data []byte, store *s3store.S3Store) string {
	// Create tus client with small chunk size to force multipart
	headers := make(http.Header)
	headers.Set("Tus-Resumable", "1.0.0")
	headers.Set("Content-Type", "application/offset+octet-stream")

	client, err := tusclient.NewClient(serverURL+"/files", &tusclient.Config{
		ChunkSize:  512,   // 512 bytes chunks to force multipart
		Resume:     false, // No resumable uploads for tests
		Store:      nil,   // No persistent storage for tests
		Header:     headers,
		HttpClient: &http.Client{},
	})
	require.NoError(t, err)

	// Create upload with metadata
	metadata := tusclient.Metadata{
		"filename": "test-upload.txt",
		"type":     "text/plain",
	}
	upload := tusclient.NewUploadFromBytes(data)
	upload.Metadata = metadata

	// Create the upload on the server and get uploader
	uploader, err := client.CreateUpload(upload)
	require.NoError(t, err)

	// Upload the data
	err = uploader.Upload()
	require.NoError(t, err)

	// Verify upload is complete by checking S3 directly
	ctx := context.Background()
	parts := strings.Split(uploader.Url(), "/")
	uploadID := parts[len(parts)-1]

	// Get the upload from the store to verify it's finalized
	tusUpload, err := store.GetUpload(ctx, uploadID)
	require.NoError(t, err)

	info, err := tusUpload.GetInfo(ctx)
	require.NoError(t, err)

	// Ensure the upload reports as complete (offset equals size)
	assert.Equal(t, info.Size, info.Offset, "Upload should be complete (offset equals size)")

	// Return the upload URL
	return uploader.Url()
}

// getUploadFromStore retrieves an upload from the S3 store by URL
func getUploadFromStore(store *s3store.S3Store, uploadURL string) (handler.Upload, error) {
	ctx := context.Background()

	// Extract upload ID from URL
	// URL format: http://host:port/files/uploadId
	parts := strings.Split(uploadURL, "/")
	uploadID := parts[len(parts)-1]

	return store.GetUpload(ctx, uploadID)
}

func TestTUSUploadReader_MultipartRangeHandling(t *testing.T) {
	// Setup TUS server with HTTP endpoint for real client uploads
	_, store, _, _, serverURL := setupTUSServer(t)
	ctx := context.Background()

	// Create test data that simulates multipart upload (larger than chunk size)
	chunk1 := []byte("This is the first chunk of data that should be larger than 512 bytes to force real multipart uploads.")
	chunk2 := []byte("This is the second chunk of data that continues the multipart upload process.")
	chunk3 := []byte("And this is the third and final chunk that completes our test data for multipart range handling.")
	fullData := bytes.Join([][]byte{chunk1, chunk2, chunk3}, nil)

	// Upload data using real TUS client with HTTP (this will use chunks/multipart)
	uploadURL := uploadWithTusClient(t, serverURL, fullData, store)

	// Get the upload from the store
	upload, err := getUploadFromStore(store, uploadURL)
	require.NoError(t, err)

	info, err := upload.GetInfo(ctx)
	require.NoError(t, err)

	// Test reading from different start positions
	testCases := []struct {
		name     string
		start    int64
		expected []byte
	}{
		{
			name:     "Read from beginning",
			start:    0,
			expected: fullData,
		},
		{
			name:     "Read from middle of first chunk",
			start:    50,
			expected: fullData[50:],
		},
		{
			name:     "Read from start of second chunk",
			start:    int64(len(chunk1)),
			expected: bytes.Join([][]byte{chunk2, chunk3}, nil),
		},
		{
			name:     "Read from middle of second chunk",
			start:    int64(len(chunk1) + 50),
			expected: fullData[int64(len(chunk1)+50):],
		},
		{
			name:     "Read from start of third chunk",
			start:    int64(len(chunk1) + len(chunk2)),
			expected: chunk3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader, err := NewTUSUploadReader(ctx, &core.Logger{Logger: zap.NewNop()}, upload, info, tc.start)
			require.NoError(t, err)

			var buf bytes.Buffer
			n, err := io.Copy(&buf, reader)
			assert.NoError(t, err)
			assert.Equal(t, int64(len(tc.expected)), n)
			assert.Equal(t, tc.expected, buf.Bytes())
			defer func() { require.NoError(t, reader.Close()) }()
		})
	}
}

func TestTUSUploadReader_SeekOperations(t *testing.T) {
	// Setup TUS server with HTTP endpoint for real client uploads
	_, store, _, _, serverURL := setupTUSServer(t)
	ctx := context.Background()

	// Create test data (larger than chunk size)
	testData := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

	// Upload data using real TUS client with HTTP (this will use chunks/multipart)
	uploadURL := uploadWithTusClient(t, serverURL, testData, store)

	// Get the upload from the store
	upload, err := getUploadFromStore(store, uploadURL)
	require.NoError(t, err)

	info, err := upload.GetInfo(ctx)
	require.NoError(t, err)

	reader, err := NewTUSUploadReader(ctx, &core.Logger{Logger: zap.NewNop()}, upload, info, 0)
	require.NoError(t, err)

	// Test SEEK_SET
	pos, err := reader.Seek(50, io.SeekStart)
	assert.NoError(t, err)
	assert.Equal(t, int64(50), pos)

	buf := make([]byte, 20)
	n, err := reader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 20, n)
	assert.Equal(t, []byte("opqrstuvwxyz01234567"), buf)

	// Test SEEK_CUR
	pos, err = reader.Seek(-10, io.SeekCurrent)
	assert.NoError(t, err)
	assert.Equal(t, int64(60), pos)

	buf = make([]byte, 10)
	n, err = reader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, []byte("yz01234567"), buf)

	// Test SEEK_END
	pos, err = reader.Seek(-15, io.SeekEnd)
	assert.NoError(t, err)
	assert.Equal(t, int64(int64(len(testData))-15), pos)

	buf = make([]byte, 20)
	n, err = reader.Read(buf)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 15, n)
	assert.Equal(t, testData[len(testData)-15:], buf[:n])
	defer func() { require.NoError(t, reader.Close()) }()
}

func TestTUSUploadReader_BufferTruncation(t *testing.T) {
	// Setup TUS server with HTTP endpoint for real client uploads
	_, store, _, _, serverURL := setupTUSServer(t)
	ctx := context.Background()

	// Create test data
	testData := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

	// Upload data using real TUS client with HTTP (this will use chunks/multipart)
	uploadURL := uploadWithTusClient(t, serverURL, testData, store)

	// Get the upload from the store
	upload, err := getUploadFromStore(store, uploadURL)
	require.NoError(t, err)

	info, err := upload.GetInfo(ctx)
	require.NoError(t, err)

	reader, err := NewTUSUploadReader(ctx, &core.Logger{Logger: zap.NewNop()}, upload, info, 0)
	require.NoError(t, err)

	// Test reading with small buffer that truncates data
	smallBuf := make([]byte, 10)

	// First read - should get first 10 bytes
	n, err := reader.Read(smallBuf)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, []byte("0123456789"), smallBuf)

	// Second read - should get next 10 bytes
	n, err = reader.Read(smallBuf)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, []byte("ABCDEFGHIJ"), smallBuf)

	// Seek to middle and read with truncating buffer
	_, err = reader.Seek(30, io.SeekStart)
	assert.NoError(t, err)

	n, err = reader.Read(smallBuf)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, []byte("UVWXYZabcd"), smallBuf)

	// Verify position tracking is correct after truncation
	pos, err := reader.Seek(0, io.SeekCurrent)
	assert.NoError(t, err)
	assert.Equal(t, int64(40), pos)

	// Close reader manually at the end
	require.NoError(t, reader.Close())
}

func TestTUSUploadReader_ReadAt(t *testing.T) {
	_, store, _, _, serverURL := setupTUSServer(t)
	ctx := context.Background()

	testData := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

	uploadURL := uploadWithTusClient(t, serverURL, testData, store)
	upload, err := getUploadFromStore(store, uploadURL)
	require.NoError(t, err)

	info, err := upload.GetInfo(ctx)
	require.NoError(t, err)

	reader, err := NewTUSUploadReader(ctx, &core.Logger{Logger: zap.NewNop()}, upload, info, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })

	buf := make([]byte, 10)

	t.Run("read from beginning", func(t *testing.T) {
		n, err := reader.ReadAt(buf, 0)
		assert.NoError(t, err)
		assert.Equal(t, 10, n)
		assert.Equal(t, []byte("0123456789"), buf)
	})

	t.Run("read from middle", func(t *testing.T) {
		n, err := reader.ReadAt(buf, 10)
		assert.NoError(t, err)
		assert.Equal(t, 10, n)
		assert.Equal(t, []byte("ABCDEFGHIJ"), buf)
	})

	t.Run("read near end with EOF", func(t *testing.T) {
		off := int64(len(testData) - 5)
		smallBuf := make([]byte, 10)
		n, err := reader.ReadAt(smallBuf, off)
		assert.Equal(t, io.EOF, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, testData[off:], smallBuf[:n])
	})

	t.Run("read at exact end returns EOF", func(t *testing.T) {
		smallBuf := make([]byte, 1)
		_, err := reader.ReadAt(smallBuf, int64(len(testData)))
		assert.Equal(t, io.EOF, err)
	})

	t.Run("does not affect position", func(t *testing.T) {
		pos, err := reader.Seek(20, io.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(20), pos)

		smallBuf := make([]byte, 5)
		_, _ = reader.ReadAt(smallBuf, 0)
		assert.Equal(t, []byte("01234"), smallBuf)

		pos, err = reader.Seek(0, io.SeekCurrent)
		assert.NoError(t, err)
		assert.Equal(t, int64(20), pos, "ReadAt must not change position")
	})
}

func TestTUSUploadReader_ReadAt_Concurrent(t *testing.T) {
	_, store, _, _, serverURL := setupTUSServer(t)
	ctx := context.Background()

	testData := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")

	uploadURL := uploadWithTusClient(t, serverURL, testData, store)
	upload, err := getUploadFromStore(store, uploadURL)
	require.NoError(t, err)

	info, err := upload.GetInfo(ctx)
	require.NoError(t, err)

	reader, err := NewTUSUploadReader(ctx, &core.Logger{Logger: zap.NewNop()}, upload, info, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })

	type result struct {
		off   int64
		data  []byte
		err   error
		n     int
	}

	offsets := []int64{0, 10, 26, 50}
	ch := make(chan result, len(offsets))

	for _, off := range offsets {
		go func(off int64) {
			buf := make([]byte, 10)
			n, err := reader.ReadAt(buf, off)
			ch <- result{off: off, data: buf[:n], err: err, n: n}
		}(off)
	}

	for range offsets {
		res := <-ch
		assert.NoError(t, res.err, "offset %d", res.off)
		assert.Equal(t, 10, res.n, "offset %d", res.off)
		assert.Equal(t, testData[res.off:res.off+10], res.data, "offset %d", res.off)
	}
}

func TestTUSUploadReader_ReadAt_AfterClose(t *testing.T) {
	_, store, _, _, serverURL := setupTUSServer(t)
	ctx := context.Background()

	testData := []byte("0123456789ABCDEF")

	uploadURL := uploadWithTusClient(t, serverURL, testData, store)
	upload, err := getUploadFromStore(store, uploadURL)
	require.NoError(t, err)

	info, err := upload.GetInfo(ctx)
	require.NoError(t, err)

	reader, err := NewTUSUploadReader(ctx, &core.Logger{Logger: zap.NewNop()}, upload, info, 0)
	require.NoError(t, err)

	require.NoError(t, reader.Close())

	buf := make([]byte, 10)
	_, err = reader.ReadAt(buf, 0)
	assert.Equal(t, io.EOF, err)
}

func TestTUSUploadReader_LargeFileRanges(t *testing.T) {
	// Setup TUS server with HTTP endpoint for real client uploads
	_, store, _, _, serverURL := setupTUSServer(t)
	ctx := context.Background()

	// Create larger test data to simulate real multipart upload
	var largeData []byte
	for i := 0; i < 100; i++ {
		line := fmt.Sprintf("Line %04d: This is test line content for a larger file to test range requests with multipart uploads.\n", i+1)
		largeData = append(largeData, []byte(line)...)
	}

	// Upload data using real TUS client with HTTP (this will use chunks/multipart)
	uploadURL := uploadWithTusClient(t, serverURL, largeData, store)

	// Get the upload from the store
	upload, err := getUploadFromStore(store, uploadURL)
	require.NoError(t, err)

	info, err := upload.GetInfo(ctx)
	require.NoError(t, err)

	// Test various ranges to simulate real-world usage
	testRanges := []struct {
		name     string
		start    int64
		expected []byte
	}{
		{
			name:     "Read from start",
			start:    0,
			expected: largeData[:500], // First 500 bytes
		},
		{
			name:     "Read from middle",
			start:    2000,
			expected: largeData[2000:2500], // 500 bytes from middle
		},
		{
			name:     "Read from near end",
			start:    int64(len(largeData)) - 500,
			expected: largeData[len(largeData)-500:], // Last 500 bytes
		},
	}

	for _, tr := range testRanges {
		t.Run(tr.name, func(t *testing.T) {
			reader, err := NewTUSUploadReader(ctx, &core.Logger{Logger: zap.NewNop()}, upload, info, tr.start)
			require.NoError(t, err)

			buf := make([]byte, 500)
			n, err := io.ReadFull(reader, buf)
			assert.NoError(t, err)
			assert.Equal(t, 500, n)
			assert.Equal(t, tr.expected, buf)
			defer func() { require.NoError(t, reader.Close()) }()
		})
	}
}
