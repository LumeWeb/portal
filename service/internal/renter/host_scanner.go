package renter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/go-units"
	"github.com/jamestrandung/go-concurrency/v2/async"
	"github.com/samber/lo"
	"go.lumeweb.com/portal/config"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/renterd/autopilot/contractor"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/jamestrandung/go-concurrency/v2/workerpool"
	"go.lumeweb.com/portal/core"
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

type HostScanner struct {
	renter     core.RenterService
	hostAPIURL string
	config     config.Manager
	logger     *core.Logger
	// Host cache
	hostCache map[types.PublicKey]hostCacheEntry
	evalState *EvaluationState
}

// ScanResult represents the outcome of scanning a single host
type ScanResult struct {
	PublicKey types.PublicKey
	Success   bool
	Error     error
	Attempts  int
}

type BatchResult struct {
	hostCount int             // Total number of hosts in batch
	newHosts  []HostscoreHost // New hosts discovered in this batch
}

type HostscoreHostResponse struct {
	Hosts []HostscoreHost `json:"hosts"`
}

type HostscoreHost struct {
	ID           int                  `json:"id"`
	Rank         int                  `json:"rank"`
	PublicKey    types.PublicKey      `json:"publicKey"`
	FirstSeen    time.Time            `json:"firstSeen"`
	KnownSince   uint64               `json:"knownSince"`
	NetAddress   string               `json:"netaddress"`
	Blocked      bool                 `json:"blocked"`
	IPNets       []string             `json:"ipNets"`
	LastIPChange time.Time            `json:"lastIPChange"`
	Settings     rhpv2.HostSettings   `json:"settings"`
	PriceTable   rhpv3.HostPriceTable `json:"priceTable"`
}

func (r BatchResult) isEmpty() bool {
	return r.hostCount == 0
}

func NewBatchResult(hosts []HostscoreHost, newHosts []HostscoreHost) BatchResult {
	return BatchResult{
		hostCount: len(hosts),
		newHosts:  newHosts,
	}
}

type ScanState struct {
	scannedHosts map[types.PublicKey]HostscoreHost
	mutex        sync.RWMutex
}

func NewScanState() *ScanState {
	return &ScanState{
		scannedHosts: make(map[types.PublicKey]HostscoreHost),
	}
}

func (s *ScanState) AddHosts(hosts []HostscoreHost) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	lo.ForEach(hosts, func(host HostscoreHost, _ int) {
		s.scannedHosts[host.PublicKey] = host
	})
}

// GetScannedHosts returns all currently tracked hosts in a slice
func (s *ScanState) GetScannedHosts() []HostscoreHost {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return lo.Values(s.scannedHosts)
}

// GetScannedHostCount returns total number of tracked hosts
func (s *ScanState) GetScannedHostCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.scannedHosts)
}

type hostCacheEntry struct {
	host   api.Host
	exists bool  // true if host exists, false if known to not exist
	err    error // if fetch failed with error
}

type EvaluationState struct {
	config         api.AutopilotConfig
	consensusState api.ConsensusState
	fee            types.Currency
	redundancy     api.RedundancySettings
	renter         core.RenterService
	// Host cache
	hostCache map[types.PublicKey]hostCacheEntry
	cacheMu   sync.RWMutex
}

func NewEvaluationState(config api.AutopilotConfig, consensusState api.ConsensusState,
	fee types.Currency, redundancy api.RedundancySettings, renter core.RenterService) *EvaluationState {
	return &EvaluationState{
		config:         config,
		consensusState: consensusState,
		fee:            fee,
		redundancy:     redundancy,
		renter:         renter,
		hostCache:      make(map[types.PublicKey]hostCacheEntry),
	}
}

func (e *EvaluationState) getCachedHost(ctx context.Context, pubKey types.PublicKey) (api.Host, error) {
	// Try cache first
	e.cacheMu.RLock()
	if entry, ok := e.hostCache[pubKey]; ok {
		e.cacheMu.RUnlock()
		if !entry.exists {
			return api.Host{}, fmt.Errorf("host known to not exist: %v", entry.err)
		}
		return entry.host, nil
	}
	e.cacheMu.RUnlock()

	// Not in cache, need to fetch
	host, err := e.renter.Host(ctx, pubKey)

	// Cache the result
	e.cacheMu.Lock()
	if err != nil {
		e.hostCache[pubKey] = hostCacheEntry{
			exists: false,
			err:    err,
		}
	} else {
		e.hostCache[pubKey] = hostCacheEntry{
			host:   host,
			exists: true,
		}
	}
	e.cacheMu.Unlock()

	if err != nil {
		return api.Host{}, err
	}
	return host, nil
}

func (e *EvaluationState) EvaluateHosts(settings api.GougingSettings, hosts []HostscoreHost) (api.ConfigEvaluationResponse, error) {
	// Convert HostscoreHosts to api.Hosts using cached lookups
	apiHosts := make([]api.Host, 0, len(hosts))
	for _, hscHost := range hosts {
		rhost, err := e.getCachedHost(context.Background(), hscHost.PublicKey)
		if err != nil {
			continue
		}
		apiHosts = append(apiHosts, rhost)
	}

	if len(apiHosts) == 0 {
		return api.ConfigEvaluationResponse{}, fmt.Errorf("no hosts available to evaluate")
	}

	return contractor.EvaluateConfig(e.config, e.consensusState, e.fee, e.redundancy, settings, apiHosts)
}

type ScanTracker map[types.PublicKey]struct{}
type AllowlistTracker map[types.PublicKey]struct{}

func NewHostScanner(ctx core.Context) *HostScanner {
	return &HostScanner{
		renter:     core.GetService[core.RenterService](ctx, core.RENTER_SERVICE),
		hostAPIURL: ctx.Config().Config().Core.Storage.Sia.HostScoreAPIURL,
		config:     ctx.Config(),
		logger:     ctx.Logger(),
		hostCache:  make(map[types.PublicKey]hostCacheEntry),
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

	requiredHosts := cfg.Contracts.Amount
	if requiredHosts == 0 {
		logger.Info("No hosts required based on current configuration")
		return nil
	}

	// Initialize evaluation state
	if err := s.initializeEvalState(ctx); err != nil {
		return fmt.Errorf("failed to initialize evaluation state: %w", err)
	}

	// Setup initial gouging settings
	if err := s.setupGougingSettings(ctx); err != nil {
		return fmt.Errorf("failed to setup gouging settings: %w", err)
	}

	// Get current gouging settings to use as base
	baseSettings, err := s.renter.GougingSettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch base gouging settings: %w", err)
	}

	// Initialize scan state and price tracking
	scanState := NewScanState()
	priceTracking := NewPriceTracking(baseSettings, requiredHosts, s.evalState, s.logger)

	// Process hosts until we find optimal configuration
	finalSettings, usableHosts, err := s.findOptimalSettings(ctx, priceTracking, scanState)
	if err != nil {
		return fmt.Errorf("failed to find optimal settings: %w", err)
	}

	// Update gouging settings with final configuration
	if err := s.renter.UpdateGougingSettings(ctx, finalSettings); err != nil {
		return fmt.Errorf("failed to update final gouging settings: %w", err)
	}

	// Final verification using TestAutoPilotConfig
	evalResp, err := s.renter.TestAutoPilotConfig(ctx, finalSettings)
	if err != nil {
		return fmt.Errorf("final verification failed: %w", err)
	}

	if evalResp.Usable < requiredHosts {
		return fmt.Errorf("final verification shows insufficient usable hosts: got %d, need %d", evalResp.Usable, requiredHosts)
	}

	s.logPriceSettings(logger, finalSettings, usableHosts)
	return nil
}

func (s *HostScanner) initializeEvalState(ctx core.Context) error {
	cs, err := s.renter.ConsensusState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get consensus state: %w", err)
	}

	cfg, err := s.renter.AutopilotConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get autopilot config: %w", err)
	}

	rs, err := s.renter.RedundancySettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get redundancy settings: %w", err)
	}

	fee, err := s.renter.RecommendedFee(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recommended fee: %w", err)
	}

	s.evalState = NewEvaluationState(cfg, cs, fee, rs, s.renter)

	return nil
}

func (s *HostScanner) findOptimalSettings(ctx core.Context, priceTracking *PriceTracking, scanState *ScanState) (api.GougingSettings, uint64, error) {
	logger := ctx.Logger()

	existingHosts, err := s.initializeAllowlist(ctx)
	if err != nil {
		return api.GougingSettings{}, 0, err
	}

	// Phase 1: Gather initial hosts
	settings, usableCount, err := s.gatherInitialHosts(ctx, priceTracking, existingHosts, scanState, logger)
	if err != nil {
		return api.GougingSettings{}, 0, err
	}

	// Verify we have minimum required hosts before optimization
	if usableCount < priceTracking.GetRequiredHosts() {
		return api.GougingSettings{}, 0, fmt.Errorf("insufficient usable hosts found: got %d, need %d",
			usableCount, priceTracking.GetRequiredHosts())
	}

	// Phase 2: Optimize prices with existing host set
	finalSettings, finalUsableCount, err := s.optimizePrices(ctx, priceTracking, logger)
	if err != nil {
		// Fall back to previous valid settings since they met requirements
		return settings, usableCount, nil
	}

	return finalSettings, finalUsableCount, nil
}

func (s *HostScanner) gatherInitialHosts(
	ctx core.Context,
	priceTracking *PriceTracking,
	existingHosts map[types.PublicKey]struct{},
	scanState *ScanState,
	logger *core.Logger,
) (api.GougingSettings, uint64, error) {
	var lastWorkingConfig api.GougingSettings
	var lastUsableCount uint64
	page := 1

	for {
		batchResult, err := s.processNextBatch(ctx, page, priceTracking, existingHosts, scanState)
		if err != nil {
			if lastUsableCount > 0 {
				return lastWorkingConfig, lastUsableCount, nil
			}
			return api.GougingSettings{}, 0, err
		}

		if batchResult.isEmpty() {
			if lastUsableCount > 0 {
				return lastWorkingConfig, lastUsableCount, nil
			}
			return api.GougingSettings{}, 0, fmt.Errorf("insufficient hosts found: %d", lastUsableCount)
		}

		// Get current price settings
		currentSettings := priceTracking.ComputeMaxPrices()

		// Evaluate all hosts we have so far
		evalResp, err := s.evaluateHosts(ctx, priceTracking.GetAllHosts(), currentSettings, priceTracking)
		if err != nil {
			if lastUsableCount > 0 {
				return lastWorkingConfig, lastUsableCount, nil
			}
			return api.GougingSettings{}, 0, fmt.Errorf("failed to evaluate hosts: %w", err)
		}

		if evalResp.Usable > lastUsableCount {
			lastWorkingConfig = currentSettings
			lastUsableCount = evalResp.Usable
		}

		if evalResp.Usable >= priceTracking.GetRequiredHosts() {
			return lastWorkingConfig, lastUsableCount, nil
		}

		logger.Info("Gathering hosts",
			zap.Int("page", page),
			zap.Int("newHostsFound", len(batchResult.newHosts)),
			zap.Uint64("usableHosts", evalResp.Usable),
			zap.Uint64("requiredHosts", priceTracking.GetRequiredHosts()),
			zap.Int("totalScannedHosts", len(priceTracking.GetAllHosts())))

		page++
	}
}

func (s *HostScanner) optimizePrices(
	ctx context.Context,
	priceTracking *PriceTracking,
	logger *core.Logger,
) (api.GougingSettings, uint64, error) {
	var bestSettings api.GougingSettings
	var bestUsableCount uint64

	priceTracking.currentPhase = PhaseInitial
	fullRedundancyTarget := uint64(float64(priceTracking.baselineRequired) * FullRedundancyMultiplier)
	minRedundancyRequired := uint64(float64(priceTracking.baselineRequired) * MinRedundancyMultiplier)

	for {
		settings, usableHosts, done, err := s.tryOptimizeSettings(ctx, priceTracking, logger)
		if err != nil {
			if bestUsableCount >= minRedundancyRequired {
				break
			}
			return api.GougingSettings{}, 0, err
		}

		if usableHosts >= minRedundancyRequired {
			if bestUsableCount == 0 || usableHosts > bestUsableCount {
				bestSettings = settings
				bestUsableCount = usableHosts
			}
		}

		if done {
			break
		}
	}

	if bestUsableCount < minRedundancyRequired {
		return api.GougingSettings{}, 0, fmt.Errorf(
			"failed to find enough hosts with minimum redundancy: got %d, need %d",
			bestUsableCount, minRedundancyRequired)
	}

	logger.Info("Initial optimization complete",
		zap.Uint64("usableHosts", bestUsableCount),
		zap.Uint64("targetHosts", fullRedundancyTarget),
		zap.Uint64("minRequired", minRedundancyRequired))

	// Second phase: Optimize download prices while maintaining minimum redundancy
	priceTracking.currentPhase = PhaseDownloadOptimization
	downloadOptimizedSettings := priceTracking.optimizeDownloadPrice(bestSettings)

	// Evaluate final settings
	evalResp, err := s.evaluateHosts(ctx, priceTracking.GetAllHosts(), downloadOptimizedSettings, priceTracking)
	if err != nil {
		logger.Info("Download optimization failed, reverting to initial settings",
			zap.Error(err))
		return bestSettings, bestUsableCount, nil
	}

	// Verify we maintain minimum redundancy
	if evalResp.Usable < minRedundancyRequired {
		logger.Info("Download optimization resulted in too few hosts, reverting to initial settings",
			zap.Uint64("usableHosts", evalResp.Usable),
			zap.Uint64("minRequired", minRedundancyRequired))
		return bestSettings, bestUsableCount, nil
	}

	priceDiff := bestSettings.MaxDownloadPrice.Sub(downloadOptimizedSettings.MaxDownloadPrice)
	logger.Info("Price optimization complete",
		zap.String("downloadPriceReduction", priceDiff.String()),
		zap.Uint64("originalUsableHosts", bestUsableCount),
		zap.Uint64("finalUsableHosts", evalResp.Usable),
		zap.Uint64("minRequired", minRedundancyRequired))

	return downloadOptimizedSettings, evalResp.Usable, nil
}

func (s *HostScanner) tryOptimizeSettings(
	ctx context.Context,
	priceTracking *PriceTracking,
	logger *core.Logger,
) (api.GougingSettings, uint64, bool, error) {
	optimizedSettings, midRank, canOptimizeFurther := priceTracking.ComputeOptimizedPrices()

	hostsToEvaluate := priceTracking.getHostsUpToRank(midRank)
	evalResp, err := s.evaluateHosts(ctx, hostsToEvaluate, optimizedSettings, priceTracking)
	if err != nil {
		return api.GougingSettings{}, 0, false, fmt.Errorf("failed to evaluate config: %w", err)
	}

	// Use phase-aware host requirements with constants
	var rankWorked bool
	if priceTracking.currentPhase == PhaseInitial {
		requiredHosts := uint64(float64(priceTracking.baselineRequired) * FullRedundancyMultiplier)
		rankWorked = evalResp.Usable >= requiredHosts
	} else {
		requiredHosts := uint64(float64(priceTracking.baselineRequired) * MinRedundancyMultiplier)
		rankWorked = evalResp.Usable >= requiredHosts
	}

	priceTracking.UpdateOptimizationResult(rankWorked, midRank, evalResp.Usable)

	// Logging...
	return optimizedSettings, evalResp.Usable, !canOptimizeFurther, nil
}

func (s *HostScanner) evaluateHosts(
	ctx context.Context,
	hscHosts []HostscoreHost,
	settings api.GougingSettings,
	priceTracking *PriceTracking,
) (api.ConfigEvaluationResponse, error) {
	hostsToRemove := make([]types.PublicKey, 0)
	hostsToEvaluate := make([]HostscoreHost, 0, len(hscHosts))

	for _, hscHost := range hscHosts {
		// Just check if we can get the host
		_, err := s.getCachedHost(ctx, hscHost.PublicKey)
		if err != nil {
			s.logger.Debug("Failed to fetch host",
				zap.String("hostKey", hscHost.PublicKey.String()),
				zap.Error(err))
			hostsToRemove = append(hostsToRemove, hscHost.PublicKey)
			continue
		}
		hostsToEvaluate = append(hostsToEvaluate, hscHost)
	}

	// If any hosts failed to fetch, remove them from consideration
	if len(hostsToRemove) > 0 {
		priceTracking.RemoveHosts(hostsToRemove)
	}

	if len(hostsToEvaluate) == 0 {
		return api.ConfigEvaluationResponse{}, fmt.Errorf("no hosts available to evaluate after filtering")
	}

	// Use evalState for actual evaluation
	return s.evalState.EvaluateHosts(settings, hostsToEvaluate)
}

func (s *HostScanner) testCurrentSettings(ctx context.Context,
	priceTracking *PriceTracking) (api.GougingSettings, uint64, error) {

	currentSettings := priceTracking.ComputeMaxPrices()
	evalResp, err := s.renter.TestAutoPilotConfig(ctx, currentSettings)
	if err != nil {
		return api.GougingSettings{}, 0, fmt.Errorf("failed to test current settings: %w", err)
	}

	return currentSettings, evalResp.Usable, nil
}

func (s *HostScanner) processNextBatch(ctx core.Context, page int,
	priceTracking *PriceTracking, existingHosts map[types.PublicKey]struct{},
	scanState *ScanState) (BatchResult, error) {

	hosts, err := s.fetchHostsPage(ctx, page)
	if err != nil {
		return BatchResult{}, fmt.Errorf("failed to fetch api.Hosts page %d: %w", page, err)
	}

	if len(hosts) == 0 {
		return BatchResult{}, nil
	}

	// Filter out blocked hosts
	validHosts := lo.Filter(hosts, func(h HostscoreHost, _ int) bool {
		return !h.Blocked
	})

	// Scan all hosts in batch
	scanResults := s.scanHostsBatch(ctx, validHosts)

	// Get successfully scanned hosts
	successfulHosts := lo.Filter(validHosts, func(h HostscoreHost, i int) bool {
		return scanResults[i].Success
	})

	// Add to scan state tracking
	scanState.AddHosts(successfulHosts)

	// Run autopilot to compute scores
	if err := s.manageScan(ctx); err != nil {
		return BatchResult{}, fmt.Errorf("failed to run autopilot scan: %w", err)
	}

	// Update price tracking with successful hosts
	priceTracking.UpdatePrices(successfulHosts)

	// Find new hosts among successful ones
	newHosts := lo.Filter(successfulHosts, func(h HostscoreHost, _ int) bool {
		_, exists := existingHosts[h.PublicKey]
		return !exists
	})

	if len(newHosts) > 0 {
		if err := s.addNewHostsToAllowlist(ctx, newHosts, existingHosts); err != nil {
			return BatchResult{}, err
		}
	}

	return NewBatchResult(hosts, newHosts), nil
}

func (s *HostScanner) addNewHostsToAllowlist(ctx context.Context, newHosts []HostscoreHost, existingHosts map[types.PublicKey]struct{}) error {
	// Extract just the public keys
	newHostKeys := lo.Map(newHosts, func(h HostscoreHost, _ int) types.PublicKey {
		return h.PublicKey
	})

	// Add to renterd's allowlist
	if err := s.renter.AddHostsToAllowlist(ctx, newHostKeys); err != nil {
		return fmt.Errorf("failed to update host allowlist: %w", err)
	}

	// Update our tracking map
	for _, pk := range newHostKeys {
		existingHosts[pk] = struct{}{}
	}

	s.logger.Info("Added new hosts to allowlist",
		zap.Int("newHostsAdded", len(newHostKeys)))

	return nil
}

func (s *HostScanner) handleEmptyBatch(config api.GougingSettings, usableCount, requiredHosts uint64) (api.GougingSettings, uint64, error) {
	if usableCount >= requiredHosts {
		return config, usableCount, nil
	}
	return api.GougingSettings{}, 0, fmt.Errorf("insufficient hosts found: %d", usableCount)
}

func (s *HostScanner) initializeAllowlist(ctx context.Context) (map[types.PublicKey]struct{}, error) {
	allowlistedHosts, err := s.renter.GetAllowlistedHosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get allowlisted hosts: %w", err)
	}

	existingHosts := make(map[types.PublicKey]struct{})
	for _, pk := range allowlistedHosts {
		existingHosts[pk] = struct{}{}
	}

	return existingHosts, nil
}

func (s *HostScanner) processNewHosts(ctx context.Context, newHosts []HostscoreHost, existingHosts map[types.PublicKey]struct{}) error {
	if len(newHosts) == 0 {
		return nil
	}

	// Extract public keys
	newHostKeys := lo.Map(newHosts, func(h HostscoreHost, _ int) types.PublicKey {
		return h.PublicKey
	})

	// Add to allowlist
	if err := s.renter.AddHostsToAllowlist(ctx, newHostKeys); err != nil {
		return fmt.Errorf("failed to update host allowlist: %w", err)
	}

	// Update tracking
	for _, pk := range newHostKeys {
		existingHosts[pk] = struct{}{}
	}

	// Scan new hosts
	s.scanHostsBatch(ctx, newHosts)

	return nil
}

// scanSingleHost attempts to scan a single host
func (s *HostScanner) scanSingleHost(ctx context.Context, hostKey types.PublicKey, hostIP string) ScanResult {
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
func (s *HostScanner) scanHostsBatch(ctx context.Context, hosts []HostscoreHost) []ScanResult {
	results := make([]ScanResult, 0, len(hosts))
	var mutex sync.Mutex

	// Create a worker pool with max 10 concurrent workers
	pool := workerpool.NewWorkerPool(
		workerpool.WithMaxSize(10),
		workerpool.WithIdleTimeout(5*time.Second),
	)
	defer pool.StopWait() // Ensure we wait for all tasks to complete

	// Create a wait group to track when all scans are complete
	var wg sync.WaitGroup
	wg.Add(len(hosts))

	// Submit scan tasks for each host
	for _, host := range hosts {
		host := host // Create local copy for closure

		// Create a task for scanning a single host
		scanTask := async.NewSilentTask(func(taskCtx context.Context) error {
			defer wg.Done()

			// Perform the scan
			result := s.scanSingleHost(taskCtx, host.PublicKey, host.NetAddress)

			// Safely append the result
			mutex.Lock()
			results = append(results, result)
			mutex.Unlock()

			// Log the result
			if result.Error != nil {
				s.logger.Debug("Host scan failed after all retries",
					zap.String("hostKey", host.PublicKey.String()),
					zap.Int("attempts", result.Attempts),
					zap.Error(result.Error))
			} else {
				s.logger.Debug("Host scan completed",
					zap.String("hostKey", host.PublicKey.String()),
					zap.Int("attempts", result.Attempts))
			}

			return nil
		})

		// Submit the task to the worker pool
		pool.Submit(ctx, scanTask)
	}

	// Wait for all scans to complete
	wg.Wait()

	return results
}

func (s *HostScanner) finalizeOptimization(ctx core.Context, config api.GougingSettings) error {
	if err := s.renter.UpdateGougingSettings(ctx, config); err != nil {
		return fmt.Errorf("failed to update final gouging settings: %w", err)
	}

	if err := s.manageScan(ctx); err != nil {
		return fmt.Errorf("failed to manage final autopilot scan: %w", err)
	}

	return nil
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

func (s *HostScanner) fetchHostsPage(ctx context.Context, page int) ([]HostscoreHost, error) {
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
		return nil, fmt.Errorf("failed to fetch api.Hosts: %w", err)
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

	var hostResponse HostscoreHostResponse
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if err := json.Unmarshal(data, &hostResponse); err != nil {
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

func (s *HostScanner) getCachedHost(ctx context.Context, pubKey types.PublicKey) (api.Host, error) {
	return s.evalState.getCachedHost(ctx, pubKey)
}
