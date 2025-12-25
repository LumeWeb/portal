package storage

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap/zaptest"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	testBucket = "test-bucket"

	// Realistic chunk sizes for production scenarios
	smallChunkSize  = 1 * 1024  // 1KB - for structured files
	mediumChunkSize = 8 * 1024  // 8KB - for general file access
	largeChunkSize  = 64 * 1024 // 64KB - for streaming reads
	testChunkSize   = 50        // Small chunks for testing chunked behavior

	// Realistic buffer sizes
	smallBuffer  = 256  // Small read buffer
	mediumBuffer = 4096 // Standard read buffer
)

func setupMockS3(t *testing.T) (string, *s3.Client, func()) {
	tempDir, err := os.MkdirTemp("", "s3-test-")
	require.NoError(t, err)

	backend, err := s3afero.SingleBucket(testBucket, afero.NewBasePathFs(afero.NewOsFs(), tempDir), nil)
	require.NoError(t, err)
	faker := gofakes3.New(backend)
	server := httptest.NewServer(faker.Server())

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			ctx, span := core.TraceMethod(ctx, "anonymous")
			defer span.End()

			return aws.Credentials{
				AccessKeyID:     "FAKEACCESSKEY",
				SecretAccessKey: "FAKESECRETKEY",
				SessionToken:    "",
				Source:          "test",
			}, nil
		})),
	)
	require.NoError(t, err)

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	cleanup := func() {
		server.Close()
		os.RemoveAll(tempDir)
	}

	return server.URL, client, cleanup
}

func createTestLogger(t *testing.T) *core.Logger {
	logger := zaptest.NewLogger(t)
	return &core.Logger{Logger: logger}
}

// createTestObject uploads test data to S3 and returns the key
func createTestObject(t *testing.T, client *s3.Client, key, data string) {
	_, err := client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(data),
	})
	require.NoError(t, err)
}

// newTestReader creates a new S3Reader with common parameters
func newTestReader(t *testing.T, client *s3.Client, key string, chunkSize int) *S3Reader {
	reader, err := NewS3Reader(
		context.Background(),
		createTestLogger(t),
		client,
		testBucket,
		key,
		&FixedChunkSizePolicy{Size: chunkSize},
	)
	require.NoError(t, err)
	return reader
}

func TestNewS3Reader(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	logger := createTestLogger(t)

	tests := []struct {
		name            string
		logger          *core.Logger
		s3client        *s3.Client
		bucket          string
		key             string
		chunkSizePolicy ChunkSizePolicy
		wantErr         bool
		errMsg          string
	}{
		{
			name:            "valid parameters",
			logger:          logger,
			s3client:        client,
			bucket:          testBucket,
			key:             "test-key",
			chunkSizePolicy: &FixedChunkSizePolicy{Size: smallChunkSize},
			wantErr:         false,
		},
		{
			name:            "nil logger",
			logger:          nil,
			s3client:        client,
			bucket:          testBucket,
			key:             "test-key",
			chunkSizePolicy: &FixedChunkSizePolicy{Size: smallChunkSize},
			wantErr:         true,
			errMsg:          "logger cannot be nil",
		},
		{
			name:            "nil s3client",
			logger:          logger,
			s3client:        nil,
			bucket:          testBucket,
			key:             "test-key",
			chunkSizePolicy: &FixedChunkSizePolicy{Size: smallChunkSize},
			wantErr:         true,
			errMsg:          "s3client cannot be nil",
		},
		{
			name:            "nil chunkSizePolicy",
			logger:          logger,
			s3client:        client,
			bucket:          testBucket,
			key:             "test-key",
			chunkSizePolicy: nil,
			wantErr:         true,
			errMsg:          "chunkSizePolicy cannot be nil",
		},
		{
			name:            "empty bucket",
			logger:          logger,
			s3client:        client,
			bucket:          "",
			key:             "test-key",
			chunkSizePolicy: &FixedChunkSizePolicy{Size: smallChunkSize},
			wantErr:         true,
			errMsg:          "bucket cannot be empty",
		},
		{
			name:            "empty key",
			logger:          logger,
			s3client:        client,
			bucket:          testBucket,
			key:             "",
			chunkSizePolicy: &FixedChunkSizePolicy{Size: smallChunkSize},
			wantErr:         true,
			errMsg:          "key cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := NewS3Reader(context.Background(), tt.logger, tt.s3client, tt.bucket, tt.key, tt.chunkSizePolicy)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, reader)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, reader)
			}
		})
	}
}

func TestS3Reader_Read(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	testData := strings.Repeat("This is a test file for S3Reader streaming functionality. ", 10)
	createTestObject(t, client, "test-file.txt", testData)

	reader := newTestReader(t, client, "test-file.txt", largeChunkSize)
	defer require.NoError(t, reader.Close())

	// Read data using realistic streaming pattern
	var result strings.Builder
	buf := make([]byte, mediumBuffer)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
	}

	// Verify all data was read correctly
	assert.Equal(t, testData, result.String())
	assert.Equal(t, len(testData), result.Len())
}

func TestS3Reader_RandomAccessPattern(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	testData := strings.Repeat("0123456789", 100) // 1000 bytes
	createTestObject(t, client, "random-file.bin", testData)

	reader := newTestReader(t, client, "random-file.bin", testChunkSize)
	defer require.NoError(t, reader.Close())

	// Simulate random access pattern
	accessPattern := []struct {
		offset int64
		size   int
		expect string
	}{
		{0, 10, "0123456789"},           // Start
		{100, 10, "0123456789"},         // Middle
		{500, 10, "0123456789"},         // Later middle
		{50, 5, "56789"},                // Back to middle
		{990, 10, "6789"},               // Near end (partial read)
		{0, 20, "01234567890123456789"}, // Back to start
	}

	for _, access := range accessPattern {
		t.Run(fmt.Sprintf("access_%d_%d", access.offset, access.size), func(t *testing.T) {
			// Seek to position
			_, err := reader.Seek(access.offset, io.SeekStart)
			require.NoError(t, err)

			// Read data
			buf := make([]byte, access.size)
			n, err := io.ReadFull(reader, buf)

			// For reads near the end, we might get EOF
			if access.offset+int64(access.size) > int64(len(testData)) {
				expectedRead := len(testData) - int(access.offset)
				assert.Equal(t, expectedRead, n)
				assert.Error(t, io.ErrUnexpectedEOF, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, access.size, n)
			}

			// Verify the content (up to what we actually read)
			if n > 0 {
				expectedData := testData[access.offset : access.offset+int64(n)]
				assert.Equal(t, expectedData, string(buf[:n]))
			}
		})
	}
}

func TestS3Reader_ReadInChunks(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	testData := strings.Repeat("0123456789", 20) // 200 bytes
	createTestObject(t, client, "large-file.txt", testData)

	reader := newTestReader(t, client, "large-file.txt", testChunkSize)
	defer require.NoError(t, reader.Close())

	// Read in small chunks to test chunked reading
	var result strings.Builder
	buf := make([]byte, 30) // Small buffer to force multiple reads

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
	}

	assert.Equal(t, testData, result.String())
}

func TestS3Reader_Seek(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	fileContent := "HEADER:File metadata and configuration information\n" +
		"DATA_START:Actual content begins here\n" +
		strings.Repeat("This is sample file content line with various data entries. ", 50) +
		"DATA_END:End of actual content\n" +
		"FOOTER:Checksum and summary information"

	createTestObject(t, client, "structured-file.txt", fileContent)

	reader := newTestReader(t, client, "structured-file.txt", mediumChunkSize)
	defer require.NoError(t, reader.Close())

	// Test realistic seek patterns used in applications
	tests := []struct {
		name     string
		testFunc func(t *testing.T, r *S3Reader, content string)
	}{
		{
			name: "read_header",
			testFunc: func(t *testing.T, r *S3Reader, content string) {
				// Seek to start and read header
				pos, err := r.Seek(0, io.SeekStart)
				require.NoError(t, err)
				assert.Equal(t, int64(0), pos)

				buf := make([]byte, 50)
				n, err := r.Read(buf)
				require.NoError(t, err)
				assert.True(t, n > 0)
				assert.Contains(t, string(buf[:n]), "HEADER:")
			},
		},
		{
			name: "skip_to_data_section",
			testFunc: func(t *testing.T, r *S3Reader, content string) {
				// Find and seek to data section
				dataStart := strings.Index(content, "DATA_START")
				require.Greater(t, dataStart, 0)

				pos, err := r.Seek(int64(dataStart), io.SeekStart)
				require.NoError(t, err)
				assert.Equal(t, int64(dataStart), pos)

				buf := make([]byte, 30)
				n, err := r.Read(buf)
				require.NoError(t, err)
				assert.True(t, n > 0)
				assert.Contains(t, string(buf[:n]), "DATA_START")
			},
		},
		{
			name: "read_from_middle",
			testFunc: func(t *testing.T, r *S3Reader, content string) {
				// Seek to middle of content (common for large files)
				middleOffset := int64(len(content) / 2)
				pos, err := r.Seek(middleOffset, io.SeekStart)
				require.NoError(t, err)
				assert.Equal(t, middleOffset, pos)

				buf := make([]byte, 100)
				n, err := r.Read(buf)
				require.NoError(t, err)
				assert.True(t, n > 0)
			},
		},
		{
			name: "read_footer",
			testFunc: func(t *testing.T, r *S3Reader, content string) {
				// Seek to footer area (common for reading metadata)
				footerStart := strings.LastIndex(content, "FOOTER:")
				require.Greater(t, footerStart, 0)

				pos, err := r.Seek(int64(footerStart), io.SeekStart)
				require.NoError(t, err)
				assert.Equal(t, int64(footerStart), pos)

				buf := make([]byte, 50)
				n, err := r.Read(buf)
				require.NoError(t, err)
				assert.True(t, n > 0)
				assert.Contains(t, string(buf[:n]), "FOOTER:")
			},
		},
		{
			name: "relative_seek",
			testFunc: func(t *testing.T, r *S3Reader, content string) {
				// Start at data section, then seek forward relatively
				dataStart := strings.Index(content, "DATA_START")
				require.Greater(t, dataStart, 0)

				pos, err := r.Seek(int64(dataStart), io.SeekStart)
				require.NoError(t, err)

				// Seek forward 100 bytes from current position
				pos, err = r.Seek(100, io.SeekCurrent)
				require.NoError(t, err)
				assert.Equal(t, int64(dataStart+100), pos)

				buf := make([]byte, 20)
				n, err := r.Read(buf)
				require.NoError(t, err)
				assert.True(t, n > 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			freshReader := newTestReader(t, client, "structured-file.txt", mediumChunkSize)
			defer freshReader.Close()
			tt.testFunc(t, freshReader, fileContent)
		})
	}
}

func TestS3Reader_SeekBackwards(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	// Create realistic log file test data
	header := "HEADER:Log file v1.0\n"
	entries := make([]string, 20)
	for i := 0; i < 20; i++ {
		entries[i] = fmt.Sprintf("ENTRY%03d:Sample log entry data with timestamp and info\n", i)
	}
	footer := "FOOTER:End of log file\n"
	testData := header + strings.Join(entries, "") + footer

	createTestObject(t, client, "logfile.txt", testData)

	reader := newTestReader(t, client, "logfile.txt", smallChunkSize)
	defer require.NoError(t, reader.Close())

	// Realistic scenario: Read through file, then go back to re-read header
	buf := make([]byte, smallBuffer)

	// Read first part of file (simulate scanning entries)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	require.Greater(t, n, 0)
	firstRead := string(buf[:n])
	assert.Contains(t, firstRead, "HEADER:")

	// Continue reading to get into the data section
	n, err = reader.Read(buf)
	require.NoError(t, err)
	require.Greater(t, n, 0)

	// Now seek backwards to re-read the header (common pattern for validation)
	pos, err := reader.Seek(0, io.SeekStart)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), pos)

	// Read header again to verify it
	headerBuf := make([]byte, len(header))
	n, err = reader.Read(headerBuf)
	require.NoError(t, err)
	assert.Equal(t, len(header), n)
	assert.Equal(t, header, string(headerBuf))

	// Another realistic scenario: Seek to middle, then backup a bit
	middlePos := int64(len(testData) / 2)
	pos, err = reader.Seek(middlePos, io.SeekStart)
	assert.NoError(t, err)
	assert.Equal(t, middlePos, pos)

	// Read a bit from the middle
	n, err = reader.Read(buf[:50])
	require.NoError(t, err)
	require.Greater(t, n, 0)

	// Now seek backwards 100 bytes (common when re-parsing a section)
	pos, err = reader.Seek(-100, io.SeekCurrent)
	assert.NoError(t, err)

	// Calculate expected position (middlePos + 50 bytes read - 100 bytes backup)
	expectedBackupPos := middlePos + 50 - 100
	if expectedBackupPos < 0 {
		expectedBackupPos = 0
	}
	assert.Equal(t, expectedBackupPos, pos)

	// Read again and verify we're at the right backup position
	n, err = reader.Read(buf[:100])
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	// Verify we can see content that should be at this backup position
	expectedContent := testData[expectedBackupPos : expectedBackupPos+int64(n)]
	assert.Equal(t, expectedContent, string(buf[:n]))
}

func TestS3Reader_ContextCancellation(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	testData := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	createTestObject(t, client, "context-test.txt", testData)

	// Create cancelable context
	ctx, cancel := context.WithCancel(context.Background())

	reader, err := NewS3Reader(
		ctx,
		createTestLogger(t),
		client,
		testBucket,
		"context-test.txt",
		&FixedChunkSizePolicy{Size: testChunkSize},
	)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Try to read - should fail with context cancelled
	buf := make([]byte, 10)
	_, err = reader.Read(buf)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	// Close should still work
	err = reader.Close()
	assert.NoError(t, err)
}

func TestFixedChunkSizePolicy(t *testing.T) {
	policy := &FixedChunkSizePolicy{Size: smallChunkSize}
	assert.Equal(t, smallChunkSize, policy.ChunkSize())
}

func TestS3Reader_FileNotExists(t *testing.T) {
	_, client, cleanup := setupMockS3(t)
	defer cleanup()

	reader := newTestReader(t, client, "nonexistent-file.txt", smallChunkSize)
	defer require.NoError(t, reader.Close())

	// Try to read from non-existent file
	buf := make([]byte, 10)
	_, err := reader.Read(buf)
	assert.Error(t, err)
}
