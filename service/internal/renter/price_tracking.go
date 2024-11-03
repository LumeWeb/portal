package renter

import (
	"fmt"
	"github.com/docker/go-units"
	"github.com/samber/lo"
	"go.lumeweb.com/portal/core"
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/api"
	"go.uber.org/zap"
	"sort"
)

const (
	PhaseInitial              = "initial"
	PhaseDownloadOptimization = "download_optimization"
	FullRedundancyMultiplier  = 1.5 // Standard redundancy for initial phase
	MinRedundancyMultiplier   = 1.2 // Minimum redundancy during download optimization
)

type PriceTracking struct {
	storagePrices  []types.Currency
	uploadPrices   []types.Currency
	downloadPrices []types.Currency
	rankedHosts    []HostscoreHost
	baseSettings   api.GougingSettings
	logger         *core.Logger

	// Core requirements
	baselineRequired uint64 // Original required hosts count
	targetHosts      uint64 // Target with full redundancy (1.5x)
	validHostsCount  uint64
	currentPhase     string // Tracks current optimization phase

	// Binary search state
	lowRank         int
	highRank        int
	bestRank        int
	bestUsableHosts uint64

	// Evaluation interface
	evalState *EvaluationState
}

func NewPriceTracking(baseSettings api.GougingSettings, requiredHosts uint64, evalState *EvaluationState, logger *core.Logger) *PriceTracking {
	targetHosts := uint64(float64(requiredHosts) * recommendedHostMultiplier)
	return &PriceTracking{
		storagePrices:    make([]types.Currency, 0),
		uploadPrices:     make([]types.Currency, 0),
		downloadPrices:   make([]types.Currency, 0),
		rankedHosts:      make([]HostscoreHost, 0),
		baseSettings:     baseSettings,
		logger:           logger,
		baselineRequired: requiredHosts,
		targetHosts:      targetHosts,
		validHostsCount:  0,
		currentPhase:     PhaseInitial,
		lowRank:          int(targetHosts),
		highRank:         int(targetHosts),
		bestRank:         int(targetHosts),
		evalState:        evalState,
	}
}

func (pt *PriceTracking) UpdatePrices(hosts []HostscoreHost) {
	// Track all hosts
	pt.rankedHosts = append(pt.rankedHosts, hosts...)

	// Ensure hosts remain sorted by rank
	sort.Slice(pt.rankedHosts, func(i, j int) bool {
		return pt.rankedHosts[i].Rank < pt.rankedHosts[j].Rank
	})

	// Update high rank bound based on total available hosts
	if len(pt.rankedHosts) < int(pt.targetHosts) {
		pt.highRank = len(pt.rankedHosts)
	} else {
		pt.highRank = int(pt.targetHosts)
	}

	// Track prices for stats/logging
	for _, host := range hosts {
		// Storage price
		storageBase := host.PriceTable.WriteStoreCost
		pt.storagePrices = append(pt.storagePrices, storageBase)

		// Upload price calculation
		uploadPerByte := host.PriceTable.UploadBandwidthCost.Add(
			host.PriceTable.WriteLengthCost)
		uploadTotal := uploadPerByte.Mul64(1e12)
		pt.uploadPrices = append(pt.uploadPrices, uploadTotal)

		// Download price calculation
		downloadPerByte := host.PriceTable.DownloadBandwidthCost.Add(
			host.PriceTable.ReadLengthCost)
		downloadTotal := downloadPerByte.Mul64(1e12)
		pt.downloadPrices = append(pt.downloadPrices, downloadTotal)
	}

	pt.validHostsCount += uint64(len(hosts))
}

func (pt *PriceTracking) ComputeMaxPrices() api.GougingSettings {
	return pt.computePricesForRank(pt.bestRank)
}

func (pt *PriceTracking) ComputeOptimizedPrices() (api.GougingSettings, int, bool) {
	pt.logger.Debug("Binary search state",
		zap.Int("low", pt.lowRank),
		zap.Int("high", pt.highRank),
		zap.Int("best", pt.bestRank),
		zap.Uint64("bestUsable", pt.bestUsableHosts),
		zap.Uint64("required", pt.baselineRequired),
		zap.Uint64("target", pt.targetHosts))

	if pt.highRank <= pt.lowRank {
		settings := pt.computePricesForRank(pt.bestRank)
		settings = pt.optimizeDownloadPrice(settings)
		return settings, pt.bestRank, false
	}

	midRank := pt.lowRank + (pt.highRank-pt.lowRank)/2
	pt.logger.Debug("Trying rank", zap.Int("midRank", midRank))

	settings := pt.computePricesForRank(midRank)
	return settings, midRank, true
}

func (pt *PriceTracking) optimizeDownloadPrice(settings api.GougingSettings) api.GougingSettings {
	pt.currentPhase = PhaseDownloadOptimization
	defer func() { pt.currentPhase = PhaseInitial }()

	pt.logger.Info("Starting download price optimization")
	pt.logPriceSettings(settings, "Initial settings")

	bestSettings := settings
	bestUsableCount := uint64(0)

	// First find hosts that meet storage and upload requirements
	storageUploadSettings := settings
	storageUploadSettings.MaxDownloadPrice = types.MaxCurrency

	baselineEvalResp, err := pt.evalState.EvaluateHosts(storageUploadSettings, pt.rankedHosts[:pt.bestRank])
	if err != nil {
		return settings
	}

	validHostCount := baselineEvalResp.Usable
	minRedundancyRequired := uint64(float64(pt.baselineRequired) * MinRedundancyMultiplier)

	pt.logger.Info("Baseline evaluation complete",
		zap.Uint64("validHosts", validHostCount),
		zap.Uint64("minRequired", minRedundancyRequired))

	// Optimize download price
	lowPrice := types.ZeroCurrency
	highPrice := settings.MaxDownloadPrice

	iterations := 0
	for highPrice.Cmp(lowPrice) > 0 && iterations < 20 {
		iterations++
		midPrice := highPrice.Add(lowPrice).Div64(2)
		testSettings := settings
		testSettings.MaxDownloadPrice = midPrice

		evalResp, err := pt.evalState.EvaluateHosts(testSettings, pt.rankedHosts[:pt.bestRank])
		if err != nil {
			lowPrice = midPrice
			pt.logger.Debug("Evaluation failed",
				zap.Int("iteration", iterations),
				zap.Error(err))
			continue
		}

		pt.logPriceSettings(testSettings, fmt.Sprintf(
			"Testing iteration %d (usable: %d/%d required, min redundancy: %d)",
			iterations, evalResp.Usable, pt.baselineRequired, minRedundancyRequired))

		if evalResp.Usable >= minRedundancyRequired {
			if bestUsableCount == 0 || evalResp.Usable < bestUsableCount {
				bestSettings = testSettings
				bestUsableCount = evalResp.Usable
			}
			highPrice = midPrice
		} else {
			lowPrice = midPrice
		}

		// Check if prices are very close (within 1% difference)
		diff := highPrice.Sub(lowPrice)
		if diff.Cmp(lowPrice.Div64(100)) <= 0 {
			break
		}
	}

	if bestUsableCount < minRedundancyRequired {
		pt.logger.Info("Download price optimization failed to maintain minimum redundancy, reverting to original settings",
			zap.Uint64("bestUsableCount", bestUsableCount),
			zap.Uint64("minRequired", minRedundancyRequired))
		return settings
	}

	downloadSavings := settings.MaxDownloadPrice.Sub(bestSettings.MaxDownloadPrice)
	pt.logger.Info("Download price optimization complete",
		zap.String("priceReduction", downloadSavings.String()),
		zap.Uint64("usableHosts", bestUsableCount))

	return bestSettings
}

func (pt *PriceTracking) HasEnoughHosts(usableHosts uint64) bool {
	switch pt.currentPhase {
	case PhaseDownloadOptimization:
		// During download optimization, we allow a lower redundancy
		minRedundancy := uint64(float64(pt.baselineRequired) * MinRedundancyMultiplier)
		return usableHosts >= minRedundancy
	default:
		// For initial host selection, maintain full redundancy
		return usableHosts >= pt.targetHosts
	}
}

func (pt *PriceTracking) UpdateOptimizationResult(rankWorked bool, testedRank int, usableHosts uint64) {
	pt.logger.Debug("Updating optimization result",
		zap.Bool("rankWorked", rankWorked),
		zap.Int("testedRank", testedRank),
		zap.Uint64("usableHosts", usableHosts))

	if rankWorked {
		// For general host selection we want to get close to target
		if pt.bestUsableHosts == 0 || usableHosts < pt.bestUsableHosts {
			pt.bestRank = testedRank
			pt.bestUsableHosts = usableHosts
		}
		pt.highRank = testedRank
	} else {
		pt.lowRank = testedRank + 1
	}

	pt.logger.Debug("Updated optimization bounds",
		zap.Int("lowRank", pt.lowRank),
		zap.Int("highRank", pt.highRank),
		zap.Int("bestRank", pt.bestRank),
		zap.Uint64("bestUsableHosts", pt.bestUsableHosts))
}

func (pt *PriceTracking) computePricesForRank(rank int) api.GougingSettings {
	settings := pt.baseSettings

	hostsToConsider := pt.getHostsUpToRank(rank)

	maxStoragePrice := types.ZeroCurrency
	maxUploadPrice := types.ZeroCurrency
	maxDownloadPrice := types.ZeroCurrency

	for _, host := range hostsToConsider {
		// Storage price
		storageBase := host.PriceTable.WriteStoreCost
		if storageBase.Cmp(maxStoragePrice) > 0 {
			maxStoragePrice = storageBase
		}

		// Upload price
		uploadPerByte := host.PriceTable.UploadBandwidthCost.Add(
			host.PriceTable.WriteLengthCost)
		uploadTotal := uploadPerByte.Mul64(1e12)
		if uploadTotal.Cmp(maxUploadPrice) > 0 {
			maxUploadPrice = uploadTotal
		}

		// Download price
		downloadPerByte := host.PriceTable.DownloadBandwidthCost.Add(
			host.PriceTable.ReadLengthCost)
		downloadTotal := downloadPerByte.Mul64(1e12)
		if downloadTotal.Cmp(maxDownloadPrice) > 0 {
			maxDownloadPrice = downloadTotal
		}
	}

	settings.MaxStoragePrice = maxStoragePrice
	settings.MaxUploadPrice = maxUploadPrice
	settings.MaxDownloadPrice = maxDownloadPrice

	return settings
}

// Helper functions

func (pt *PriceTracking) getHostsUpToRank(rankCutoff int) []HostscoreHost {
	if len(pt.rankedHosts) <= rankCutoff {
		return pt.rankedHosts
	}
	return pt.rankedHosts[:rankCutoff]
}

func (pt *PriceTracking) GetCurrentRankCutoff() int {
	return pt.bestRank
}

func (pt *PriceTracking) GetValidHostCount() uint64 {
	return pt.validHostsCount
}

func (pt *PriceTracking) GetRequiredHosts() uint64 {
	if pt.currentPhase == PhaseDownloadOptimization {
		return uint64(float64(pt.baselineRequired) * MinRedundancyMultiplier)
	}
	return uint64(float64(pt.baselineRequired) * FullRedundancyMultiplier)
}

func (pt *PriceTracking) GetSearchBounds() (low, high, best int) {
	return pt.lowRank, pt.highRank, pt.bestRank
}

func (pt *PriceTracking) GetAllHosts() []HostscoreHost {
	return pt.rankedHosts
}

func (pt *PriceTracking) RemoveHosts(toRemove []types.PublicKey) {
	removeSet := make(map[types.PublicKey]struct{})
	for _, pk := range toRemove {
		removeSet[pk] = struct{}{}
	}

	pt.rankedHosts = lo.Filter(pt.rankedHosts, func(h HostscoreHost, _ int) bool {
		_, shouldRemove := removeSet[h.PublicKey]
		return !shouldRemove
	})

	if len(pt.rankedHosts) < int(pt.targetHosts) {
		pt.highRank = len(pt.rankedHosts)
	}
}

func (pt *PriceTracking) logPriceSettings(settings api.GougingSettings, msg string) {
	storagePrice := siacoinsToRat(settings.MaxStoragePrice)
	storagePrice = ratMultiply(storagePrice, blocksPerMonth)
	storagePrice = ratMultiply(storagePrice, units.TB)

	downloadPrice := siacoinsToRat(settings.MaxDownloadPrice)
	uploadPrice := siacoinsToRat(settings.MaxUploadPrice)

	pt.logger.Debug(msg,
		zap.String("storage", storagePrice.FloatString(decimalsInSiacoin)),
		zap.String("download", downloadPrice.FloatString(decimalsInSiacoin)),
		zap.String("upload", uploadPrice.FloatString(decimalsInSiacoin)))
}
