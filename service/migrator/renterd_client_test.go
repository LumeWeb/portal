package migrator

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenterdClient_ListObjects verifies that the client calls the correct
// bus endpoint: GET /api/bus/objects/* with bucket and pagination as query params.
func TestRenterdClient_ListObjects(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBucket, capturedMarker, capturedLimit string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedBucket = r.URL.Query().Get("bucket")
		capturedMarker = r.URL.Query().Get("marker")
		capturedLimit = r.URL.Query().Get("limit")

		resp := RenterdObjectsResponse{
			HasMore:    false,
			NextMarker: "",
			Objects: []RenterdObjectMetadata{
				{Bucket: "ipfs", Key: "QmTest1", Size: 42},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "test-password")
	resp, err := client.ListObjects(context.Background(), "ipfs", "")
	require.NoError(t, err)

	assert.Equal(t, "GET", capturedMethod)
	assert.Equal(t, "/api/bus/objects/", capturedPath)
	assert.Equal(t, "ipfs", capturedBucket)
	assert.Equal(t, "", capturedMarker)
	assert.Equal(t, "1000", capturedLimit)
	assert.Len(t, resp.Objects, 1)
	assert.Equal(t, "QmTest1", resp.Objects[0].Key)
}

// TestRenterdClient_ListObjects_WithMarker verifies pagination uses the
// marker query parameter (not the path prefix).
func TestRenterdClient_ListObjects_WithMarker(t *testing.T) {
	var capturedMarker, capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMarker = r.URL.Query().Get("marker")
		capturedPath = r.URL.Path
		resp := RenterdObjectsResponse{
			HasMore: false,
			Objects: []RenterdObjectMetadata{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "test-password")
	_, err := client.ListObjects(context.Background(), "ipfs", "next-page-token")
	require.NoError(t, err)

	assert.Equal(t, "next-page-token", capturedMarker)
	assert.Equal(t, "/api/bus/objects/", capturedPath)
}

// TestRenterdClient_ListObjects_AuthVerifies basic auth is set correctly.
func TestRenterdClient_ListObjects_Auth(t *testing.T) {
	var capturedUser, capturedPass string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser, capturedPass, _ = r.BasicAuth()
		resp := RenterdObjectsResponse{Objects: []RenterdObjectMetadata{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "secret-pass")
	_, err := client.ListObjects(context.Background(), "ipfs", "")
	require.NoError(t, err)
	assert.Equal(t, "", capturedUser)
	assert.Equal(t, "secret-pass", capturedPass)
}

// TestRenterdClient_ListObjects_HTTPError verifies error propagation on
// non-200 responses.
func TestRenterdClient_ListObjects_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "test-password")
	_, err := client.ListObjects(context.Background(), "ipfs", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "internal error")
}

// TestRenterdClient_ListAllObjects_Pagination verifies automatic
// pagination across multiple pages.
func TestRenterdClient_ListAllObjects_Pagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp RenterdObjectsResponse
		if callCount == 1 {
			resp = RenterdObjectsResponse{
				HasMore:    true,
				NextMarker: "page2",
				Objects: []RenterdObjectMetadata{
					{Bucket: "ipfs", Key: "obj1", Size: 1},
				},
			}
		} else {
			assert.Equal(t, "page2", r.URL.Query().Get("marker"))
			resp = RenterdObjectsResponse{
				HasMore: false,
				Objects: []RenterdObjectMetadata{
					{Bucket: "ipfs", Key: "obj2", Size: 2},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "test-password")
	objects, err := client.ListAllObjects(context.Background(), "ipfs")
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Len(t, objects, 2)
	assert.Equal(t, "obj1", objects[0].Key)
	assert.Equal(t, "obj2", objects[1].Key)
}

// TestRenterdClient_DownloadObject verifies that the client calls the correct
// worker endpoint: GET /api/worker/objects/{key}?bucket=...
func TestRenterdClient_DownloadObject(t *testing.T) {
	var capturedMethod, capturedPath, capturedBucket string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedBucket = r.URL.Query().Get("bucket")
		w.Write([]byte("file contents"))
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "test-password")
	rc, err := client.DownloadObject(context.Background(), "ipfs", "QmTest1")
	require.NoError(t, err)
	defer rc.Close()

	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "file contents", string(body))
	assert.Equal(t, "GET", capturedMethod)
	assert.Equal(t, "/api/worker/objects/QmTest1", capturedPath)
	assert.Equal(t, "ipfs", capturedBucket)
}

// TestRenterdClient_DownloadObject_EscapesKey verifies that object keys
// with special characters are properly URL-escaped in the request path.
func TestRenterdClient_DownloadObject_EscapesKey(t *testing.T) {
	var rawPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.EscapedPath() gives the raw, still-encoded path
		rawPath = r.URL.EscapedPath()
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "test-password")
	rc, err := client.DownloadObject(context.Background(), "ipfs", "/sub/dir/file name.txt")
	require.NoError(t, err)
	rc.Close()

	// Leading slash stripped, all slashes and spaces escaped by url.PathEscape
	assert.Equal(t, "/api/worker/objects/sub%2Fdir%2Ffile%20name.txt", rawPath)
}

// TestRenterdClient_DownloadObject_HTTPError verifies error propagation.
func TestRenterdClient_DownloadObject_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "test-password")
	_, err := client.DownloadObject(context.Background(), "ipfs", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "not found")
}

// TestRenterdClient_DownloadObject_AuthVerifies basic auth on worker requests.
func TestRenterdClient_DownloadObject_Auth(t *testing.T) {
	var capturedPass string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, capturedPass, _ = r.BasicAuth()
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "worker-pass")
	rc, err := client.DownloadObject(context.Background(), "ipfs", "key1")
	require.NoError(t, err)
	rc.Close()
	assert.Equal(t, "worker-pass", capturedPass)
}

// TestNewRenterdClient_TrimsTrailingSlash verifies that a trailing slash
// in the URL is stripped to avoid double-slash in API paths.
func TestNewRenterdClient_TrimsTrailingSlash(t *testing.T) {
	c := NewRenterdClient("http://localhost:9980/", "pass")
	assert.Equal(t, "http://localhost:9980", c.baseURL)

	c2 := NewRenterdClient("http://localhost:9980", "pass")
	assert.Equal(t, "http://localhost:9980", c2.baseURL)
}

// TestRenterdClient_SameURLForBusAndWorker verifies that both bus and worker
// API calls go to the same base URL with only the path prefix differing.
func TestRenterdClient_SameURLForBusAndWorker(t *testing.T) {
	var paths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/api/bus/") {
			resp := RenterdObjectsResponse{Objects: []RenterdObjectMetadata{}}
			json.NewEncoder(w).Encode(resp)
		} else {
			w.Write([]byte("data"))
		}
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "pass")
	_, err := client.ListObjects(context.Background(), "ipfs", "")
	require.NoError(t, err)

	rc, err := client.DownloadObject(context.Background(), "ipfs", "key1")
	require.NoError(t, err)
	rc.Close()

	require.Len(t, paths, 2)
	assert.True(t, strings.HasPrefix(paths[0], "/api/bus/"))
	assert.True(t, strings.HasPrefix(paths[1], "/api/worker/"))
}

// TestRenterdClient_URLEncoding verifies that bucket names with special
// characters are properly encoded in query parameters.
func TestRenterdClient_URLEncoding(t *testing.T) {
	var capturedBucket string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBucket = r.URL.Query().Get("bucket")
		resp := RenterdObjectsResponse{Objects: []RenterdObjectMetadata{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewRenterdClient(srv.URL, "pass")
	_, err := client.ListObjects(context.Background(), "my bucket", "")
	require.NoError(t, err)
	assert.Equal(t, "my bucket", capturedBucket)
}
