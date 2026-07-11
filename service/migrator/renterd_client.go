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
	HasMore    bool                  `json:"hasMore"`
	NextMarker string                `json:"nextMarker"`
	Objects    []RenterdObjectMetadata `json:"objects"`
}

// RenterdClient is a minimal HTTP client for the renterd bus and worker APIs.
// It does NOT import go.sia.tech/renterd/v2 — the dependency was removed from
// go.mod and should not be re-added for a one-time migration.
type RenterdClient struct {
	busURL    string
	workerURL string
	password  string
	client    *http.Client
}

// NewRenterdClient creates a client for the renterd bus and worker APIs.
// busURL and workerURL should include the scheme (e.g. http://host:9980).
func NewRenterdClient(busURL, workerURL, password string) *RenterdClient {
	return &RenterdClient{
		busURL:    strings.TrimSuffix(busURL, "/"),
		workerURL: strings.TrimSuffix(workerURL, "/"),
		password:  password,
		client:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// ListObjects paginates through all objects in the given bucket.
// Returns one page at a time; call again with the returned marker to get
// the next page. Empty marker + HasMore=false means all objects listed.
func (c *RenterdClient) ListObjects(ctx context.Context, bucket, marker string) (RenterdObjectsResponse, error) {
	values := url.Values{}
	values.Set("bucket", bucket)
	if marker != "" {
		values.Set("marker", marker)
	}
	values.Set("limit", "1000")

	endpoint := fmt.Sprintf("%s/objects/?%s", c.busURL, values.Encode())
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
func (c *RenterdClient) DownloadObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("bucket", bucket)

	escapedKey := url.PathEscape(strings.TrimPrefix(key, "/"))
	endpoint := fmt.Sprintf("%s/object/%s?%s", c.workerURL, escapedKey, values.Encode())

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
