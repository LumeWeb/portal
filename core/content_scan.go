package core

import (
	"context"
	"encoding/json"
	"go.lumeweb.com/portal/db/models"
	"time"
)

const CONTENT_SCANNER_SERVICE = "content_scanner"

// ScanResult represents the outcome of a content scan
type ScanResult struct {
	Passed    bool           `json:"passed"`
	Reason    string         `json:"reason,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	ScannerID string         `json:"scanner_id"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func (s *ScanResult) ToModel(hash StorageHash) *models.ScanResult {
	metaJSON, _ := json.Marshal(s.Metadata)
	return &models.ScanResult{
		Hash:      hash.Multihash(),
		ScannerID: s.ScannerID,
		Passed:    s.Passed,
		Reason:    s.Reason,
		Metadata:  metaJSON,
	}
}

// ContentScanner interface defines how scanners should behave
type ContentScanner interface {
	// ID returns unique identifier for this scanner
	ID() string

	// Name returns human-readable name
	Name() string

	// ScanContent performs the actual scan
	ScanContent(ctx context.Context, hash StorageHash) (*ScanResult, error)

	// Priority determines scan order (higher runs first)
	Priority() int
}

// ContentScannerService manages multiple scanners
type ContentScannerService interface {
	// RegisterScanner adds a new scanner
	RegisterScanner(scanner ContentScanner) error

	// ScanContent runs content through all registered scanners
	ScanContent(ctx context.Context, hash StorageHash) ([]*ScanResult, error)

	// GetScanResults retrieves previous scan results
	GetScanResults(ctx context.Context, hash StorageHash) ([]*ScanResult, error)

	Service
}

// noopScanHandler implements a no-op content scanner
type noopScanHandler struct{}

func (h *noopScanHandler) ID() string {
	return "noop"
}

func (h *noopScanHandler) Name() string {
	return "No-op Scanner"
}

func (h *noopScanHandler) Priority() int {
	return 0
}

func (h *noopScanHandler) ScanContent(ctx context.Context, hash StorageHash) (*ScanResult, error) {
	return &ScanResult{
		Passed:    true,
		Reason:    "No-op scanner always passes",
		Timestamp: time.Now(),
		ScannerID: h.ID(),
		Metadata:  map[string]any{},
	}, nil
}

func (h *noopScanHandler) ValidateRequest(ctx context.Context, req *models.Request) error {
	return nil
}

func (h *noopScanHandler) Execute(ctx context.Context, req *models.Request) error {
	return nil
}

func (h *noopScanHandler) Cleanup(ctx context.Context, req *models.Request) error {
	return nil
}

func (h *noopScanHandler) GetStatus(ctx context.Context, req *models.Request) (RequestStatus, error) {
	return RequestStatus{
		State:   string(models.RequestStatusCompleted),
		Message: "No-op scan completed",
	}, nil
}

func NewNoContentScanner() ContentScanner {
	return &noopScanHandler{}
}
