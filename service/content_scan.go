package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	contentScanMetrics "go.lumeweb.com/portal/service/internal/content_scan"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var _ core.ContentScannerService = (*ContentScannerServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.CONTENT_SCANNER_SERVICE,
		Factory: NewContentScannerService,
		Metrics: contentScanMetrics.GetCollectors(),
	})
}

// Default implementation
type ContentScannerServiceDefault struct {
	*core.BaseComponent
	scanners []core.ContentScanner
	mu       sync.RWMutex
}

func NewContentScannerService() (core.Service, []core.ContextBuilderOption, error) {
	svc := &ContentScannerServiceDefault{
		scanners: make([]core.ContentScanner, 0),
	}

	return svc, nil, nil
}

func (s *ContentScannerServiceDefault) ID() string {
	return core.CONTENT_SCANNER_SERVICE
}

func (s *ContentScannerServiceDefault) RegisterScanner(scanner core.ContentScanner) error {
	return core.MetricTrack(
		contentScanMetrics.ScanDuration.WithLabelValues(contentScanMetrics.LabelOpRegister),
		contentScanMetrics.OperationFailed.WithLabelValues(contentScanMetrics.LabelOpRegister),
		func() error {
			if scanner == nil {
				return errors.New("scanner cannot be nil")
			}

			s.mu.Lock()
			defer s.mu.Unlock()

			// Check for duplicate ID
			for _, existing := range s.scanners {
				if existing.ID() == scanner.ID() {
					return fmt.Errorf("scanner with ID %s already registered", scanner.ID())
				}
			}

			s.scanners = append(s.scanners, scanner)

			// Sort by priority
			sort.Slice(s.scanners, func(i, j int) bool {
				return s.scanners[i].Priority() > s.scanners[j].Priority()
			})

			s.Logger().Info("Registered content scanner",
				zap.String("id", scanner.ID()),
				zap.String("name", scanner.Name()),
				zap.Int("priority", scanner.Priority()))

			contentScanMetrics.ScannerRegistered.WithLabelValues(scanner.ID()).Inc()
			return nil
		},
	)
}

func (s *ContentScannerServiceDefault) ScanContent(ctx context.Context, hash core.StorageHash) ([]*core.ScanResult, error) {
	ctx, span := core.TraceMethod(ctx, "ContentScannerServiceDefault.ScanContent")
	defer span.End()

	return core.MetricTrackResult(
		contentScanMetrics.ScanDuration.WithLabelValues(contentScanMetrics.LabelOpScan),
		contentScanMetrics.OperationFailed.WithLabelValues(contentScanMetrics.LabelOpScan),
		func() ([]*core.ScanResult, error) {
			s.mu.RLock()
			scanners := append([]core.ContentScanner(nil), s.scanners...)
			s.mu.RUnlock()

			if len(scanners) == 0 {
				return nil, nil
			}

			results := make([]*core.ScanResult, 0)

			// Run each scanner in priority order
			for _, scanner := range scanners {
				result, err := scanner.ScanContent(ctx, hash)
				if err != nil {
					s.Logger().Error("Scanner failed",
						zap.String("scanner", scanner.ID()),
						zap.Error(err))
					continue
				}

				results = append(results, result)

				// Track scan metrics
				contentScanMetrics.Scanned.WithLabelValues(scanner.ID()).Inc()
				if result.Passed {
					contentScanMetrics.ScansPassed.WithLabelValues(scanner.ID()).Inc()
				} else {
					contentScanMetrics.ScansFailed.WithLabelValues(scanner.ID()).Inc()
				}

				// Store result
				if err := s.storeScanResult(ctx, hash, result); err != nil {
					s.Logger().Error("Failed to store scan result",
						zap.Error(err))
				}

				// If scanner failed content, stop processing
				if !result.Passed {
					break
				}
			}

			return results, nil
		},
	)
}

func (s *ContentScannerServiceDefault) GetScanResults(ctx context.Context, hash core.StorageHash) ([]*core.ScanResult, error) {
	ctx, span := core.TraceMethod(ctx, "ContentScannerServiceDefault.GetScanResults")
	defer span.End()

	result, err := core.MetricTrackResult(
		contentScanMetrics.ScanDuration.WithLabelValues(contentScanMetrics.LabelOpGetResults),
		contentScanMetrics.OperationFailed.WithLabelValues(contentScanMetrics.LabelOpGetResults),
		func() ([]*core.ScanResult, error) {
			var results []*core.ScanResult

			err := s.DB().Transaction(func(tx *gorm.DB) error {
				return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
					return db.WithContext(ctx).
						Model(&models.ScanResult{}).
						Where("hash = ?", hash.Multihash()).
						Order("created_at DESC").
						Find(&results)
				})
			})

			if err != nil {
				return nil, fmt.Errorf("failed to get scan results: %w", err)
			}

			return results, nil
		},
	)

	if err == nil {
		contentScanMetrics.ResultsQueried.WithLabelValues(contentScanMetrics.LabelOpGetResults).Inc()
	}
	return result, err
}

func (s *ContentScannerServiceDefault) GetScanResultById(ctx context.Context, id uint) (*core.ScanResult, error) {
	ctx, span := core.TraceMethod(ctx, "ContentScannerServiceDefault.GetScanResultById")
	defer span.End()

	result, err := core.MetricTrackResult(
		contentScanMetrics.ScanDuration.WithLabelValues(contentScanMetrics.LabelOpGetResultById),
		contentScanMetrics.OperationFailed.WithLabelValues(contentScanMetrics.LabelOpGetResultById),
		func() (*core.ScanResult, error) {
			var result core.ScanResult

			err := s.DB().Transaction(func(tx *gorm.DB) error {
				return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
					return db.WithContext(ctx).
						Model(&models.ScanResult{}).
						First(&result, id)
				})
			})

			if err != nil {
				return nil, fmt.Errorf("failed to get scan results: %w", err)
			}

			return &result, nil
		},
	)

	if err == nil {
		contentScanMetrics.ResultsQueried.WithLabelValues(contentScanMetrics.LabelOpGetResultById).Inc()
	}
	return result, err
}

func (s *ContentScannerServiceDefault) storeScanResult(ctx context.Context, hash core.StorageHash, result *core.ScanResult) error {
	ctx, span := core.TraceMethod(ctx, "ContentScannerServiceDefault.storeScanResult")
	defer span.End()

	metadataJSON, err := json.Marshal(result.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal meta %w", err)
	}

	scanResult := &models.ScanResult{
		Hash:      hash.Multihash(),
		ScannerID: result.ScannerID,
		Passed:    result.Passed,
		Reason:    result.Reason,
		Metadata:  datatypes.JSON(metadataJSON),
		Model: gorm.Model{
			CreatedAt: result.Timestamp,
		},
	}

	return s.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Create(scanResult)
		})
	})
}

func (s *ContentScannerServiceDefault) RegisteredScanners() []core.ContentScanner {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy to avoid exposing the internal backing array
	if len(s.scanners) == 0 {
		return nil
	}

	scannersCopy := make([]core.ContentScanner, len(s.scanners))
	copy(scannersCopy, s.scanners)
	return scannersCopy
}

func (s *ContentScannerServiceDefault) ClearScanners() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanners = make([]core.ContentScanner, 0)
}
