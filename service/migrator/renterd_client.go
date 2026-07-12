// Package migrator provides a one-time migration tool for moving objects
// from a renterd backend to the indexd-native backend.
//
// This package is intentionally self-contained and isolated from the rest
// of the codebase. It depends only on the portal core interfaces, the
// indexd SDK adapter, and a minimal renterd HTTP client. After migration
// is complete, this entire package should be deleted.
package migrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RenterdObjectMetadata is a subset of renterd's api.ObjectMetadata —
// only the fields the migrator needs.
type RenterdObjectMetadata struct {
	Bucket  string `json:"bucket"`
	Key     string `json:"key"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

// RenterdObjectsResponse is a subset of renterd's api.ObjectsResponse.
type RenterdObjectsResponse struct {
	HasMore    bool                    `json:"hasMore"`
	NextMarker string                  `json:"nextMarker"`
	Objects    []RenterdObjectMetadata `json:"objects"`
}

// RenterdClient is a minimal HTTP client for the renterd bus and worker APIs.
// It does NOT import go.sia.tech/renterd/v2 — the dependency was removed from
// go.mod and should not be re-added for a one-time migration.
//
// In renterd v2, the bus and worker are both served from the same address,
// just under different path prefixes: /api/bus and /api/worker.
type RenterdClient struct {
	baseURL  string
	password string
	client   *http.Client
}

// NewRenterdClient creates a client for the renterd API.
// url should include the scheme (e.g. http://host:9980).
// The bus API is at {url}/api/bus and the worker API is at {url}/api/worker.
func NewRenterdClient(url, password string) *RenterdClient {
	return &RenterdClient{
		baseURL:  strings.TrimSuffix(url, "/"),
		password: password,
		client:   &http.Client{Timeout: 5 * time.Minute},
	}
}

// objectKeyEscape replicates renterd's api.ObjectKeyEscape.
func objectKeyEscape(key string) string {
	return url.PathEscape(strings.TrimPrefix(key, "/"))
}

// ListObjects paginates through all objects in the given bucket.
// Returns one page at a time; call again with the returned marker to get
// the next page. Empty marker + HasMore=false means all objects listed.
//
// The renterd bus route is GET /api/bus/objects/*prefix where prefix is
// a path parameter used to filter by key prefix (empty = all objects).
// Pagination is handled via the marker query parameter.
func (c *RenterdClient) ListObjects(ctx context.Context, bucket, marker string) (RenterdObjectsResponse, error) {
	values := url.Values{}
	values.Set("bucket", bucket)
	if marker != "" {
		values.Set("marker", marker)
	}
	values.Set("limit", "1000")

	// Empty prefix lists all objects. The route is /objects/*prefix which
	// matches /objects/ with an empty prefix.
	endpoint := fmt.Sprintf("%s/api/bus/objects/?%s", c.baseURL, values.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return RenterdObjectsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth("", c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return RenterdObjectsResponse{}, fmt.Errorf("failed to list objects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return RenterdObjectsResponse{}, fmt.Errorf("list objects failed: %s: %s", resp.Status, string(body))
	}

	var result RenterdObjectsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return RenterdObjectsResponse{}, fmt.Errorf("failed to decode objects response: %w", err)
	}
	return result, nil
}

// ListAllObjects lists all objects in a bucket, paginating automatically.
func (c *RenterdClient) ListAllObjects(ctx context.Context, bucket string) ([]RenterdObjectMetadata, error) {
	var all []RenterdObjectMetadata
	marker := ""
	for {
		page, err := c.ListObjects(ctx, bucket, marker)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Objects...)
		if !page.HasMore {
			break
		}
		marker = page.NextMarker
	}
	return all, nil
}

// DownloadObject streams an object's data from the renterd worker.
// The caller must close the returned ReadCloser.
//
// The renterd worker route is GET /api/worker/object/*key (singular "object").
func (c *RenterdClient) DownloadObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("bucket", bucket)

	escapedKey := objectKeyEscape(key)
	endpoint := fmt.Sprintf("%s/api/worker/object/%s?%s", c.baseURL, escapedKey, values.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}
	req.SetBasicAuth("", c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download object: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: %s: %s", resp.Status, string(body))
	}

	return resp.Body, nil
}
