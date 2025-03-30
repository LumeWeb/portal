package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"sort"
)

// Default implementation
type ContentScannerServiceDefault struct {
	ctx      core.Context
	logger   *core.Logger
	db       *gorm.DB
	scanners []core.ContentScanner
}

func NewContentScannerService() (*ContentScannerServiceDefault, []core.ContextBuilderOption, error) {
	svc := &ContentScannerServiceDefault{
		scanners: make([]core.ContentScanner, 0),
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			svc.ctx = ctx
			svc.logger = ctx.ServiceLogger(svc)
			svc.db = ctx.DB()
			return nil
		}),
	)

	return svc, opts, nil
}

func (s *ContentScannerServiceDefault) ID() string {
	return core.CONTENT_SCANNER_SERVICE
}

func (s *ContentScannerServiceDefault) RegisterScanner(scanner core.ContentScanner) error {
	if scanner == nil {
		return errors.New("scanner cannot be nil")
	}

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

	s.logger.Info("Registered content scanner",
		zap.String("id", scanner.ID()),
		zap.String("name", scanner.Name()),
		zap.Int("priority", scanner.Priority()))

	return nil
}

func (s *ContentScannerServiceDefault) ScanContent(ctx context.Context, hash core.StorageHash) ([]*core.ScanResult, error) {
	if len(s.scanners) == 0 {
		return nil, nil
	}

	results := make([]*core.ScanResult, 0)

	// Run each scanner in priority order
	for _, scanner := range s.scanners {
		result, err := scanner.ScanContent(ctx, hash)
		if err != nil {
			s.logger.Error("Scanner failed",
				zap.String("scanner", scanner.ID()),
				zap.Error(err))
			continue
		}

		results = append(results, result)

		// Store result
		if err := s.storeScanResult(ctx, hash, result); err != nil {
			s.logger.Error("Failed to store scan result",
				zap.Error(err))
		}

		// If scanner failed content, stop processing
		if !result.Passed {
			break
		}
	}

	return results, nil
}

func (s *ContentScannerServiceDefault) GetScanResults(ctx context.Context, hash core.StorageHash) ([]*core.ScanResult, error) {
	var results []*core.ScanResult

	err := s.db.Transaction(func(tx *gorm.DB) error {
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
}

func (s *ContentScannerServiceDefault) storeScanResult(ctx context.Context, hash core.StorageHash, result *core.ScanResult) error {
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

	return s.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Create(scanResult)
		})
	})
}
