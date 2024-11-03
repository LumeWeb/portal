package renter

import (
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/api"
	"math"
	"sort"
)

type PriceTracking struct {
	maxStoragePrice types.Currency
	maxUploadPrice  types.Currency
	downloadPrices  []types.Currency
	validHostsCount uint64
	baseSettings    api.GougingSettings // Store the base settings
}

func NewPriceTracking(baseSettings api.GougingSettings) *PriceTracking {
	return &PriceTracking{
		maxStoragePrice: types.ZeroCurrency,
		maxUploadPrice:  types.ZeroCurrency,
		downloadPrices:  make([]types.Currency, 0),
		baseSettings:    baseSettings,
	}
}

func (pt *PriceTracking) UpdatePrices(hosts []Host) {
	for _, host := range hosts {
		storageBase := host.PriceTable.WriteStoreCost

		uploadPerByte := host.PriceTable.UploadBandwidthCost.Add(
			host.PriceTable.WriteLengthCost)
		uploadTotal := uploadPerByte.Mul64(1e12)

		downloadPerByte := host.PriceTable.DownloadBandwidthCost.Add(
			host.PriceTable.ReadLengthCost)
		downloadTotal := downloadPerByte.Mul64(1e12)

		// Update maximums
		if storageBase.Cmp(pt.maxStoragePrice) > 0 {
			pt.maxStoragePrice = storageBase
		}
		if uploadTotal.Cmp(pt.maxUploadPrice) > 0 {
			pt.maxUploadPrice = uploadTotal
		}

		pt.downloadPrices = append(pt.downloadPrices, downloadTotal)
	}

	pt.validHostsCount += uint64(len(hosts))
}

func (pt *PriceTracking) ComputeFinalPrices() api.GougingSettings {
	// Start with the base settings to preserve all non-price parameters
	settings := pt.baseSettings

	// Update only the price-related fields
	settings.MaxStoragePrice = pt.maxStoragePrice
	settings.MaxUploadPrice = pt.maxUploadPrice

	// Calculate 75th percentile for download price
	if len(pt.downloadPrices) > 0 {
		sort.Slice(pt.downloadPrices, func(i, j int) bool {
			return pt.downloadPrices[i].Cmp(pt.downloadPrices[j]) < 0
		})
		percentileIndex := int(math.Round(float64(len(pt.downloadPrices)-1) * 0.75))
		settings.MaxDownloadPrice = pt.downloadPrices[percentileIndex]
	}

	return settings
}
