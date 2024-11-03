package renter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/go-units"
	"go.lumeweb.com/portal/config"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/samber/lo"
	"go.lumeweb.com/portal/core"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/api"
	"go.uber.org/zap"
)

const (
	recommendedHostMultiplier = 1.5
	hostsPerBatch             = 50 // Number of hosts to scan at once
	blocksPerMonth            = 30 * 144
	decimalsInSiacoin         = 28
	maxRetries                = 3                // Maximum number of retry attempts
	retryDelay                = 5 * time.Second  // Delay between retry attempts
	scanTimeout               = 30 * time.Minute // Maximum time to wait for scan completion
	scanCheckInterval         = 15 * time.Second // How often to check scan status
)

// Host represents a Sia host with its settings and metadata
type Host struct {
	ID         int                  `json:"id"`
	Rank       int                  `json:"rank"`
	PublicKey  types.PublicKey      `json:"publicKey"`
	NetAddress string               `json:"netaddress"`
	Blocked    bool                 `json:"blocked"`
	PriceTable rhpv3.HostPriceTable `json:"priceTable"`
}

type HostResponse struct {
	Hosts []Host `json:"hosts"`
}

type HostScanner struct {
	renter     core.RenterService
	hostAPIURL string
	config     config.Manager
	logger     *core.Logger
}

// ScanResult represents the outcome of scanning a single host
type ScanResult struct {
	PublicKey types.PublicKey
	Success   bool
	Error     error
	Attempts  int
}

func NewHostScanner(ctx core.Context) *HostScanner {
	return &HostScanner{
		renter:     core.GetService[core.RenterService](ctx, core.RENTER_SERVICE),
		hostAPIURL: ctx.Config().Config().Core.Storage.Sia.HostScoreAPIURL,
		config:     ctx.Config(),
		logger:     ctx.Logger(),
	}
}

// ScanForHosts performs the complete host scanning workflow
func (s *HostScanner) ScanForHosts(ctx core.Context) error {
	logger := ctx.Logger()

	// Get autopilot config to determine required hosts
	cfg, err := s.renter.AutopilotConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get autopilot config: %w", err)
	}

	requiredHosts := uint64(float64(cfg.Contracts.Amount) * recommendedHostMultiplier)
	if requiredHosts == 0 {
		logger.Info("No hosts required based on current configuration")
		return nil
	}

	// Setup initial gouging settings
	if err := s.setupGougingSettings(ctx); err != nil {
		return fmt.Errorf("failed to setup gouging settings: %w", err)
	}

	// Process hosts in batches until we have enough
	canidates, err := s.processHostBatches(ctx, requiredHosts)
	if err != nil {
		return err
	}

	// Wait for any existing scan to complete and trigger a new one
	if err := s.manageScan(ctx); err != nil {
		return fmt.Errorf("failed to manage autopilot scan: %w", err)
	}

	// Log the final price settings
	gougingSettings, err := s.renter.GougingSettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch final gouging settings: %w", err)
	}

	s.logPriceSettings(logger, gougingSettings, canidates)

	return nil
}

// scanSingleHost attempts to scan a single host
func (s *HostScanner) scanSingleHost(ctx core.Context, hostKey types.PublicKey, hostIP string) ScanResult {
	result := ScanResult{
		PublicKey: hostKey,
		Success:   false,
		Attempts:  0,
	}

	// Implement retry logic
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result.Attempts = attempt

		// Try to scan the host
		_, err := s.renter.ScanHost(ctx, hostKey, hostIP)
		if err == nil {
			result.Success = true
			return result
		}

		// Log the retry attempt
		if attempt < maxRetries {
			s.logger.Debug("Host scan failed, retrying",
				zap.String("hostKey", hostKey.String()),
				zap.Int("attempt", attempt),
				zap.Error(err))

			// Wait before retrying
			time.Sleep(retryDelay)
			continue
		}

		// Final attempt failed
		result.Error = fmt.Errorf("failed to scan host after %d attempts: %w", maxRetries, err)
	}

	return result
}

// scanHostsBatch scans a batch of hosts in parallel
func (s *HostScanner) scanHostsBatch(ctx core.Context, hosts []Host) []ScanResult {
	results := make([]ScanResult, 0, len(hosts))
	var mutex sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent scans

	for _, host := range hosts {
		wg.Add(1)
		go func(h Host) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			result := s.scanSingleHost(ctx, h.PublicKey, h.NetAddress)

			mutex.Lock()
			results = append(results, result)
			mutex.Unlock()

			if result.Error != nil {
				s.logger.Debug("Host scan failed after all retries",
					zap.String("hostKey", h.PublicKey.String()),
					zap.Int("attempts", result.Attempts),
					zap.Error(result.Error))
			} else {
				s.logger.Debug("Host scan completed",
					zap.String("hostKey", h.PublicKey.String()),
					zap.Int("attempts", result.Attempts))
			}
		}(host)
	}

	wg.Wait()
	return results
}

func (s *HostScanner) processHostBatches(ctx core.Context, requiredHosts uint64) (uint64, error) {
	logger := ctx.Logger()
	page := 1

	// Get initial gouging settings to use as base
	baseSettings, err := s.renter.GougingSettings(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch base gouging settings: %w", err)
	}

	priceTracking := NewPriceTracking(baseSettings)

	// Get initial allowlisted hosts
	allowlistedHosts, err := s.renter.GetAllowlistedHosts(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get allowlisted hosts: %w", err)
	}

	// Track existing and scanned hosts
	existingHosts := make(map[types.PublicKey]struct{})
	for _, pk := range allowlistedHosts {
		existingHosts[pk] = struct{}{}
	}
	scannedHosts := make(map[types.PublicKey]struct{})

	for {
		// Fetch one batch of hosts
		hosts, err := s.fetchHostsPage(ctx, page)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch hosts page %d: %w", page, err)
		}

		if len(hosts) == 0 {
			return 0, fmt.Errorf("ran out of hosts to scan, only found %d usable hosts", priceTracking.validHostsCount)
		}

		// Filter for non-blocked hosts
		validHosts := lo.Filter(hosts, func(h Host, _ int) bool {
			return !h.Blocked
		})

		// Update price tracking with new valid hosts
		priceTracking.UpdatePrices(validHosts)

		// Filter for new hosts not in allowlist
		newHosts := lo.Filter(validHosts, func(h Host, _ int) bool {
			_, exists := existingHosts[h.PublicKey]
			return !exists
		})

		if len(newHosts) > 0 {
			// Add new hosts to allowlist
			newHostKeys := lo.Map(newHosts, func(h Host, _ int) types.PublicKey {
				return h.PublicKey
			})

			if err := s.renter.AddHostsToAllowlist(ctx, newHostKeys); err != nil {
				return 0, fmt.Errorf("failed to update host allowlist: %w", err)
			}

			// Update tracking
			for _, pk := range newHostKeys {
				existingHosts[pk] = struct{}{}
			}

			logger.Info("Added new hosts to allowlist",
				zap.Int("page", page),
				zap.Int("newHostsAdded", len(newHostKeys)))
		}

		// Find hosts that need scanning
		hostsToScan := lo.Filter(validHosts, func(h Host, _ int) bool {
			_, alreadyScanned := scannedHosts[h.PublicKey]
			return !alreadyScanned
		})

		if len(hostsToScan) > 0 {
			// Scan the hosts
			scanResults := s.scanHostsBatch(ctx, hostsToScan)

			// Update scanned hosts tracking
			for _, result := range scanResults {
				scannedHosts[result.PublicKey] = struct{}{}
			}

			successfulScans := len(lo.Filter(scanResults, func(item ScanResult, _ int) bool {
				return item.Success
			}))

			logger.Info("Completed host scans",
				zap.Int("page", page),
				zap.Int("hostsScanned", len(hostsToScan)),
				zap.Int("scannedSuccessfully", successfulScans))
		}

		// Update gouging settings for this batch
		newGougingCfg := priceTracking.ComputeFinalPrices()

		// Wait for any existing AP scan and trigger a new one
		if err := s.manageScan(ctx); err != nil {
			return 0, fmt.Errorf("failed to manage autopilot scan: %w", err)
		}

		// Log current price settings
		s.logPriceSettings(logger, newGougingCfg, priceTracking.validHostsCount)

		evalResp, err := s.renter.TestAutoPilotConfig(ctx, newGougingCfg)
		if err != nil {
			return 0, fmt.Errorf("failed to test autopilot config: %w", err)
		}

		usableHostCount := evalResp.Usable

		if usableHostCount >= requiredHosts {
			logger.Info("Found enough usable hosts",
				zap.Uint64("usable", usableHostCount),
				zap.Uint64("required", requiredHosts))
			return usableHostCount, nil
		}

		logger.Info("Need more hosts, continuing search",
			zap.Uint64("usableHosts", usableHostCount),
			zap.Uint64("required", requiredHosts))

		page++
	}
}

func (s *HostScanner) logPriceSettings(logger *core.Logger, settings api.GougingSettings, totalHosts uint64) {
	storagePrice := siacoinsToRat(settings.MaxStoragePrice)
	storagePrice = ratMultiply(storagePrice, blocksPerMonth)
	storagePrice = ratMultiply(storagePrice, units.TB)

	downloadPrice := siacoinsToRat(settings.MaxDownloadPrice)
	uploadPrice := siacoinsToRat(settings.MaxUploadPrice)


	logger.Info("Current price settings",
		zap.Uint64("hosts.total", totalHosts),
		zap.String("price.storage", storagePrice.FloatString(decimalsInSiacoin)),
		zap.String("price.download", downloadPrice.FloatString(decimalsInSiacoin)),
		zap.String("price.upload", uploadPrice.FloatString(decimalsInSiacoin)))
}

func (s *HostScanner) setupGougingSettings(ctx core.Context) error {
	// Fetch current gouging settings
	gougingSettings, err := s.renter.GougingSettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch gouging settings: %w", err)
	}

	maxContractPriceRat, err := newRat(s.config.Config().Core.Storage.Sia.MaxContractSCPrice, "max contract price")
	if err != nil {
		return fmt.Errorf("failed to parse max contract price: %w", err)
	}

	maxContractPrice, err := siacoinsFromRat(maxContractPriceRat)
	if err != nil {
		return fmt.Errorf("failed to parse max contract price: %w", err)
	}
	gougingSettings.MaxContractPrice = maxContractPrice

	maxRPCPriceRat, err := newRat(s.config.Config().Core.Storage.Sia.MaxRPCSCPrice, "max rpc price")
	if err != nil {
		return fmt.Errorf("failed to parse max rpc price: %w", err)
	}

	maxRPCPriceRat = ratDivide(maxRPCPriceRat, 1_000_000)
	maxRPCPrice, err := siacoinsFromRat(maxRPCPriceRat)
	if err != nil {
		return fmt.Errorf("failed to parse max rpc price: %w", err)
	}

	gougingSettings.MaxRPCPrice = maxRPCPrice

	// Update gouging settings
	return s.renter.UpdateGougingSettings(ctx, gougingSettings)
}

func (s *HostScanner) updateMinimumPrices(settings *api.GougingSettings, hosts []Host) {
	if len(hosts) == 0 {
		return
	}

	// Collect all download prices for statistical analysis
	downloadPrices := make([]types.Currency, 0, len(hosts))

	for _, host := range hosts {
		// Storage and Upload calculations remain the same
		storageBase := host.PriceTable.WriteStoreCost

		uploadPerByte := host.PriceTable.UploadBandwidthCost.Add(
			host.PriceTable.WriteLengthCost)
		uploadTotal := uploadPerByte.Mul64(1e12)

		// Collect download prices
		downloadPerByte := host.PriceTable.DownloadBandwidthCost.Add(
			host.PriceTable.ReadLengthCost)
		downloadTotal := downloadPerByte.Mul64(1e12)
		downloadPrices = append(downloadPrices, downloadTotal)

		// Update storage and upload maximums as before
		if storageBase.Cmp(settings.MaxStoragePrice) > 0 {
			settings.MaxStoragePrice = storageBase
		}
		if uploadTotal.Cmp(settings.MaxUploadPrice) > 0 {
			settings.MaxUploadPrice = uploadTotal
		}
	}

	// New download price calculation using percentile approach
	sort.Slice(downloadPrices, func(i, j int) bool {
		return downloadPrices[i].Cmp(downloadPrices[j]) < 0
	})

	// Use 75th percentile for download price instead of maximum
	percentileIndex := int(math.Round(float64(len(downloadPrices)-1) * 0.75))
	settings.MaxDownloadPrice = downloadPrices[percentileIndex]

	// Logging remains the same
	storagePrice := siacoinsToRat(settings.MaxStoragePrice)
	storagePrice = ratMultiply(storagePrice, blocksPerMonth)
	storagePrice = ratMultiply(storagePrice, units.TB)

	s.logger.Debug("Minimum storage price computed",
		zap.String("storagePrice", storagePrice.FloatString(decimalsInSiacoin)),
		zap.String("SC/TB/Month", storagePrice.FloatString(decimalsInSiacoin)))

	downloadPrice := siacoinsToRat(settings.MaxDownloadPrice)
	s.logger.Debug("Minimum download price computed",
		zap.String("downloadPrice", downloadPrice.FloatString(decimalsInSiacoin)),
		zap.String("SC/TB", downloadPrice.FloatString(decimalsInSiacoin)))

	uploadPrice := siacoinsToRat(settings.MaxUploadPrice)
	s.logger.Debug("Minimum upload price computed",
		zap.String("uploadPrice", uploadPrice.FloatString(decimalsInSiacoin)),
		zap.String("SC/TB", uploadPrice.FloatString(decimalsInSiacoin)))
}

func (s *HostScanner) fetchHostsPage(ctx core.Context, page int) ([]Host, error) {
	urlValues := url.Values{}
	urlValues.Add("offset", strconv.Itoa(hostsPerBatch*page))
	urlValues.Add("sort", "rank")
	urlValues.Add("order", "asc")
	urlValues.Add("limit", strconv.Itoa(hostsPerBatch))

	reqURL := fmt.Sprintf("%s/v1/hosts?%s", s.hostAPIURL, urlValues.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch hosts: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.logger.Error("failed to close response body", zap.Error(err))
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var hostResponse HostResponse
	if err := json.NewDecoder(resp.Body).Decode(&hostResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return hostResponse.Hosts, nil
}

func (s *HostScanner) manageScan(ctx core.Context) error {
	logger := ctx.Logger()

	// First check if a scan is already running
	state, err := s.renter.AutopilotState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get autopilot state: %w", err)
	}

	// If already scanning, wait for completion
	if state.Scanning {
		logger.Info("Existing autopilot scan in progress, waiting for completion")
		if err := s.waitForScanCompletion(ctx); err != nil {
			return err
		}
	}

	// Trigger new scan
	logger.Info("Triggering new autopilot scan")
	if _, err := s.renter.TriggerAutoPilot(ctx); err != nil {
		return fmt.Errorf("failed to trigger autopilot scan: %w", err)
	}

	// Wait for the new scan to complete
	logger.Info("Waiting for new scan to complete")
	if err := s.waitForScanCompletion(ctx); err != nil {
		return err
	}

	logger.Info("Autopilot scan completed successfully")
	return nil
}

func (s *HostScanner) waitForScanCompletion(ctx core.Context) error {
	logger := ctx.Logger()
	timeoutCtx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()

	ticker := time.NewTicker(scanCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for scan completion: %w", timeoutCtx.Err())
		case <-ticker.C:
			state, err := s.renter.AutopilotState(ctx)
			if err != nil {
				return fmt.Errorf("failed to get autopilot state: %w", err)
			}

			if !state.Scanning {
				return nil
			}

			logger.Debug("Waiting for scan completion")
		}
	}
}

func newRat(num string, name string) (*big.Rat, error) {
	parsedNum, ok := new(big.Rat).SetString(num)

	if !ok {
		return nil, errors.New("failed to parse " + name)
	}

	return parsedNum, nil
}

func siacoinsFromRat(r *big.Rat) (types.Currency, error) {
	r.Mul(r, new(big.Rat).SetInt(types.HastingsPerSiacoin.Big()))
	i := new(big.Int).Div(r.Num(), r.Denom())
	if i.Sign() < 0 {
		return types.ZeroCurrency, errors.New("value cannot be negative")
	} else if i.BitLen() > 128 {
		return types.ZeroCurrency, errors.New("value overflows Currency representation")
	}
	return types.NewCurrency(i.Uint64(), new(big.Int).Rsh(i, 64).Uint64()), nil
}

func siacoinsToRat(c types.Currency) *big.Rat {
	// Convert Currency to big.Int hastings
	hastings := c.Big()

	// Convert to siacoins by dividing by HastingsPerSiacoin
	return new(big.Rat).Quo(
		new(big.Rat).SetInt(hastings),
		new(big.Rat).SetInt(types.HastingsPerSiacoin.Big()),
	)
}

func ratDivide(a *big.Rat, b uint64) *big.Rat {
	return new(big.Rat).Quo(a, new(big.Rat).SetUint64(b))
}
func ratMultiply(a *big.Rat, b uint64) *big.Rat {
	return new(big.Rat).Mul(a, new(big.Rat).SetUint64(b))
}
