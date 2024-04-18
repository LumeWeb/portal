package renter

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/shopspring/decimal"

	"github.com/docker/go-units"

	"github.com/LumeWeb/portal/db/models"

	siasdk "github.com/LumeWeb/siacentral-api"

	"github.com/LumeWeb/portal/config"
	"github.com/LumeWeb/portal/cron"
	siasdksia "github.com/LumeWeb/siacentral-api/sia"
	"go.sia.tech/core/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ cron.CronableService = (*PriceTracker)(nil)

const usdSymbol = "usd"
const blocksPerMonth = 30 * 144
const decimalsInSiacoin = 28

type PriceTracker struct {
	config *config.Manager
	logger *zap.Logger
	cron   *cron.CronServiceDefault
	db     *gorm.DB
	renter *RenterDefault
	api    *siasdksia.APIClient
}

func (p PriceTracker) RegisterTasks(crn cron.CronService) error {
	crn.RegisterTask(cronTaskRecordSiaRateName, p.recordRate, cronTaskRecordSiaRateDefinition, nopArgsFactory)
	crn.RegisterTask(cronTaskImportSiaPriceHistoryName, p.importPrices, cron.TaskDefinitionOneTimeJob, nopArgsFactory)
	crn.RegisterTask(cronTaskUpdateSiaRenterPriceName, p.updatePrices, cron.TaskDefinitionOneTimeJob, nopArgsFactory)
	return nil
}

func (p PriceTracker) ScheduleJobs(crn cron.CronService) error {
	rateJob := cronTaskRecordSiaRateDefinition()

	exists, rateJobItem := crn.JobExists(cronTaskRecordSiaRateName, nil, nil)

	if !exists {
		err := crn.CreateJobScheduled(cronTaskRecordSiaRateName, nil, nil, rateJob)
		if err != nil {
			return err
		}
	} else {
		err := crn.CreateExistingJobScheduled(uuid.UUID(rateJobItem.UUID), rateJob)
		if err != nil {
			return err
		}
	}

	err := crn.CreateJobIfNotExists(cronTaskImportSiaPriceHistoryName, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p PriceTracker) recordRate(_ any) error {
	rate, _, err := p.api.GetExchangeRate()
	if err != nil {
		p.logger.Error("failed to get exchange rate", zap.Error(err))
		return err
	}

	siaPrice, ok := rate[usdSymbol]
	if !ok {
		p.logger.Error("exchange rate not found")
		return err
	}

	var history models.SCPriceHistory

	history.Rate = siaPrice

	if tx := p.db.Create(&history); tx.Error != nil {
		p.logger.Error("failed to save price history", zap.Error(tx.Error))
	}

	err = p.cron.CreateJobIfNotExists(cronTaskUpdateSiaRenterPriceName, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p PriceTracker) updatePrices(_ any) error {
	var averageRate decimal.Decimal
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

	if averageRate.Equal(decimal.Zero) {
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

	maxDownloadPrice, err := computeByRate(p.config.Config().Core.Storage.Sia.MaxDownloadPrice, averageRate, "max download price")
	if err != nil {
		return err
	}

	gouge.MaxDownloadPrice, err = siacoinsFromRat(maxDownloadPrice)
	if err != nil {
		return err
	}

	maxUploadPrice, err := computeByRate(p.config.Config().Core.Storage.Sia.MaxUploadPrice, averageRate, "max upload price")
	if err != nil {
		return err
	}

	p.logger.Debug("Setting max upload price", zap.String("maxUploadPrice", maxUploadPrice.FloatString(decimalsInSiacoin)))

	gouge.MaxUploadPrice, err = siacoinsFromRat(maxUploadPrice)
	if err != nil {
		return err
	}

	maxContractPrice, err := newRat(p.config.Config().Core.Storage.Sia.MaxContractSCPrice, "max contract price")
	if err != nil {
		return err
	}

	p.logger.Debug("Setting max contract price", zap.String("maxContractPrice", maxContractPrice.FloatString(decimalsInSiacoin)))

	gouge.MaxContractPrice, err = siacoinsFromRat(maxContractPrice)
	if err != nil {
		return err
	}

	maxRPCPrice, err := newRat(p.config.Config().Core.Storage.Sia.MaxRPCSCPrice, "max rpc price")
	if err != nil {
		return err
	}

	maxRPCPrice = ratDivide(maxRPCPrice, 1_000_000)

	p.logger.Debug("Setting max RPC price", zap.String("maxRPCPrice", maxRPCPrice.FloatString(decimalsInSiacoin)))

	gouge.MaxRPCPrice, err = siacoinsFromRat(maxRPCPrice)
	if err != nil {
		return err
	}

	maxStoragePrice, err := computeByRate(p.config.Config().Core.Storage.Sia.MaxStoragePrice, averageRate, "max storage price")
	if err != nil {
		return err
	}

	maxStoragePrice = ratDivideFloat(maxStoragePrice, redundancy.Redundancy())
	maxStoragePrice = ratDivide(maxStoragePrice, units.TiB)
	maxStoragePrice = ratDivide(maxStoragePrice, blocksPerMonth)

	p.logger.Debug("Setting max storage price", zap.String("maxStoragePrice", maxStoragePrice.FloatString(decimalsInSiacoin)))

	gouge.MaxStoragePrice, err = siacoinsFromRat(maxStoragePrice)
	if err != nil {
		return err
	}

	err = p.renter.UpdateGougingSettings(context.Background(), gouge)
	if err != nil {
		return err
	}

	err = p.cron.CreateJobIfNotExists(cronTaskUpdateSiaRenterPriceName, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p PriceTracker) importPrices(_ any) error {
	var existingDates []time.Time
	daysOfHistory := int(p.config.Config().Core.Storage.Sia.PriceHistoryDays)
	startDate := time.Now().UTC().AddDate(0, 0, -daysOfHistory)

	// Query to find which dates already have records within the last daysOfHistory days
	err := p.db.Model(&models.SCPriceHistory{}).
		Where("created_at >= ?", startDate).
		Select("DATE(created_at) as date").
		Group("DATE(created_at)").
		Order("date ASC").
		Pluck("date", &existingDates).Error

	if err != nil {
		p.logger.Error("failed to fetch existing historical dates", zap.Error(err))
		return err
	}

	existingDateMap := make(map[string]bool)
	for _, d := range existingDates {
		existingDateMap[d.Format("2006-01-02")] = true
	}

	for i := 1; i <= daysOfHistory; i++ {
		currentDate := startDate.AddDate(0, 0, i)
		dateKey := currentDate.Format("2006-01-02")
		if _, exists := existingDateMap[dateKey]; !exists {
			// Fetch and store data for currentDate as it's missing
			timestamp := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, time.UTC)
			rates, err := p.api.GetHistoricalExchangeRate(timestamp)
			if err != nil {
				p.logger.Error("failed to fetch historical exchange rate", zap.Error(err))
				return err
			}

			// Assuming USD rates as an example
			rate, exists := rates[usdSymbol]
			if !exists {
				p.logger.Error("USD rate not found for date", zap.String("date", dateKey))
				continue // Skip to the next date
			}

			priceRecord := &models.SCPriceHistory{
				Rate:      rate,
				CreatedAt: timestamp,
			}

			err = p.db.Create(&priceRecord).Error
			if err != nil {
				p.logger.Error("failed to create historical record for date", zap.String("date", dateKey), zap.Error(err))
				return err
			}
		}
	}

	err = p.cron.CreateJobIfNotExists(cronTaskUpdateSiaRenterPriceName, nil, nil)
	if err != nil {
		return err
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
	PriceApi *siasdksia.APIClient
}

func (p PriceTracker) init() error {
	p.cron.RegisterService(p)
	p.api = siasdk.NewSiaClient()

	return nil
}

func NewPriceTracker(lc fx.Lifecycle, params PriceTrackerParams) *PriceTracker {
	return &PriceTracker{
		config: params.Config,
		logger: params.Logger,
		cron:   params.Cron,
		db:     params.Db,
		renter: params.Renter,
		api:    params.PriceApi,
	}

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

func computeByRate(num string, rate decimal.Decimal, name string) (*big.Rat, error) {
	parsedNum, err := newRat(num, name)
	if err != nil {
		return nil, err
	}

	parsedRate := new(big.Rat).Quo(parsedNum, rate.Rat())

	return parsedRate, nil
}

func newRat(num string, name string) (*big.Rat, error) {
	parsedNum, ok := new(big.Rat).SetString(num)

	if !ok {
		return nil, errors.New("failed to parse " + name)
	}

	return parsedNum, nil
}

func ratDivide(a *big.Rat, b uint64) *big.Rat {
	return new(big.Rat).Quo(a, new(big.Rat).SetUint64(b))
}
func ratDivideFloat(a *big.Rat, b float64) *big.Rat {
	return new(big.Rat).Quo(a, new(big.Rat).SetFloat64(b))
}
