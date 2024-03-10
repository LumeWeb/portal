package renter

import (
	"context"
	"errors"
	"math/big"
	"time"

	"git.lumeweb.com/LumeWeb/portal/db/models"

	"github.com/siacentral/apisdkgo"

	"git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/cron"
	"github.com/go-co-op/gocron/v2"
	siasdk "github.com/siacentral/apisdkgo/sia"
	"go.sia.tech/core/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ cron.CronableService = (*PriceTracker)(nil)

const usdSymbol = "usd"

type PriceTracker struct {
	config *config.Manager
	logger *zap.Logger
	cron   *cron.CronServiceDefault
	db     *gorm.DB
	renter *RenterDefault
	api    *siasdk.APIClient
}

func (p PriceTracker) LoadInitialTasks(cron cron.CronService) error {
	job := gocron.DurationJob(time.Minute)
	_, err := cron.Scheduler().NewJob(
		job,
		gocron.NewTask(p.recordRate),
	)
	if err != nil {
		return err
	}

	return err
}

func (p PriceTracker) recordRate() {
	rate, _, err := p.api.GetExchangeRate()
	if err != nil {
		p.logger.Error("failed to get exchange rate", zap.Error(err))
		return
	}

	siaPrice, ok := rate[usdSymbol]
	if !ok {
		p.logger.Error("exchange rate not found")
		return
	}

	var history models.SCPriceHistory

	history.Rate = siaPrice

	if tx := p.db.Create(&history); tx.Error != nil {
		p.logger.Error("failed to save price history", zap.Error(tx.Error))
	}

	if err := p.updatePrices(); err != nil {
		p.logger.Error("failed to update prices", zap.Error(err))
	}
}

func (p PriceTracker) updatePrices() error {
	var averageRate float64
	days := p.config.Config().Core.Storage.Sia.PriceHistoryDays
	sql := `
SELECT AVG(rate) as average_rate FROM (
  SELECT rate FROM (
    SELECT rate, ROW_NUMBER() OVER (PARTITION BY DATE(created_at) ORDER BY created_at DESC) as rn
    FROM sc_price_history
    WHERE created_at >= NOW() - INTERVAL ? day
  ) tmp WHERE rn = 1
) final;
`
	err := p.db.Raw(sql, days).Scan(&averageRate).Error
	if err != nil {
		p.logger.Error("failed to fetch average rate", zap.Error(err), zap.Uint64("days", days))
		return err
	}

	if averageRate == 0 {
		p.logger.Error("average rate is 0")
		return errors.New("average rate is 0")
	}

	ctx := context.Background()

	gouge, err := p.renter.GougingSettings(ctx)

	if err != nil {
		p.logger.Error("failed to fetch gouging settings", zap.Error(err))
		return err
	}

	redundancy, err := p.renter.RedundancySettings(ctx)

	if err != nil {
		p.logger.Error("failed to fetch redundancy settings", zap.Error(err))
		return err
	}

	maxDownloadPrice := p.config.Config().Core.Storage.Sia.MaxDownloadPrice / averageRate
	maxDownloadPrice = maxDownloadPrice / redundancy.Redundancy()

	maxDownloadPriceSc, err := siacoinsFromFloat(maxDownloadPrice)
	if err != nil {
		return err
	}

	gouge.MaxDownloadPrice = maxDownloadPriceSc

	gouge.MaxUploadPrice, err = siacoinsFromFloat(p.config.Config().Core.Storage.Sia.MaxUploadPrice / averageRate)
	if err != nil {
		return err
	}

	gouge.MaxContractPrice, err = siacoinsFromFloat(p.config.Config().Core.Storage.Sia.MaxContractPrice / averageRate)
	if err != nil {
		return err
	}

	gouge.MaxStoragePrice, err = siacoinsFromFloat(p.config.Config().Core.Storage.Sia.MaxStoragePrice / averageRate)
	if err != nil {
		return err
	}

	gouge.MaxRPCPrice, err = siacoinsFromFloat(p.config.Config().Core.Storage.Sia.MaxRPCPrice / averageRate)
	if err != nil {
		return err
	}

	err = p.renter.UpdateGougingSettings(context.Background(), gouge)
	if err != nil {
		return err
	}

	return nil
}

func (p PriceTracker) importPrices() error {
	var count int64

	// Query to count the number of historical records
	err := p.db.Model(&models.SCPriceHistory{}).Count(&count).Error
	if err != nil {
		p.logger.Error("failed to count historical records", zap.Error(err))
		return err
	}

	daysOfHistory := p.config.Config().Core.Storage.Sia.PriceHistoryDays

	// Check if the count is less than x
	if uint64(count) < daysOfHistory {
		// Calculate how many records need to be fetched and created
		missingRecords := daysOfHistory - uint64(count)
		for i := uint64(0); i < missingRecords; i++ {
			currentDate := time.Now().UTC().AddDate(0, 0, int(-i))
			timestamp := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, time.UTC)
			// Fetch the historical exchange rate for the calculated timestamp
			rates, err := p.api.GetHistoricalExchangeRate(timestamp)
			if err != nil {
				p.logger.Error("failed to fetch historical exchange rate", zap.Error(err))
				return err
			}

			// Assuming you want to store rates for a specific currency, say "USD"
			rate, exists := rates[usdSymbol]
			if !exists {
				p.logger.Error("USD rate not found for timestamp", zap.String("timestamp", timestamp.String()))
				return errors.New("USD rate not found for timestamp")
			}

			// Create a new record in the database for each fetched rate
			priceRecord := &models.SCPriceHistory{
				Rate:      rate,
				CreatedAt: timestamp,
			}

			err = p.db.Create(&priceRecord).Error
			if err != nil {
				p.logger.Error("failed to create historical record", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

type PriceTrackerParams struct {
	fx.In
	Config   *config.Manager
	Logger   *zap.Logger
	Cron     *cron.CronServiceDefault
	Db       *gorm.DB
	Renter   *RenterDefault
	PriceApi *siasdk.APIClient
}

func (p PriceTracker) init() error {
	p.cron.RegisterService(p)
	p.api = apisdkgo.NewSiaClient()

	go func() {
		err := p.importPrices()
		if err != nil {
			p.logger.Fatal("failed to import prices", zap.Error(err))
		}

		err = p.updatePrices()
		if err != nil {
			p.logger.Fatal("failed to update prices", zap.Error(err))
		}
	}()

	return nil
}

func NewPriceTracker(params PriceTrackerParams) *PriceTracker {
	return &PriceTracker{
		config: params.Config,
		logger: params.Logger,
		cron:   params.Cron,
		db:     params.Db,
		renter: params.Renter,
		api:    params.PriceApi,
	}
}

func siacoinsFromFloat(f float64) (types.Currency, error) {
	r := new(big.Rat).SetFloat64(f)
	r.Mul(r, new(big.Rat).SetInt(types.HastingsPerSiacoin.Big()))
	i := new(big.Int).Div(r.Num(), r.Denom())
	if i.Sign() < 0 {
		return types.ZeroCurrency, errors.New("value cannot be negative")
	} else if i.BitLen() > 128 {
		return types.ZeroCurrency, errors.New("value overflows Currency representation")
	}
	return types.NewCurrency(i.Uint64(), new(big.Int).Rsh(i, 64).Uint64()), nil
}
