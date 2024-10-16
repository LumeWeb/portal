package renter

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/shopspring/decimal"

	"github.com/docker/go-units"

	"go.lumeweb.com/portal/db/models"

	siasdk "github.com/LumeWeb/siacentral-api"
	siasdksia "github.com/LumeWeb/siacentral-api/sia"
	"go.sia.tech/core/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ core.Cronable = (*PriceTracker)(nil)

const usdSymbol = "usd"
const blocksPerMonth = 30 * 144
const decimalsInSiacoin = 28

type PriceTracker struct {
	config config.Manager
	logger *core.Logger
	cron   core.CronService
	db     *gorm.DB
	renter core.RenterService
	api    *siasdksia.APIClient
}

func (p PriceTracker) RegisterTasks(crn core.CronService) error {
	crn.RegisterTask(cronTaskRecordSiaRateName, core.CronTaskFuncHandler(p.recordRate), cronTaskRecordSiaRateDefinition, nopArgsFactory, true)
	crn.RegisterTask(cronTaskImportSiaPriceHistoryName, core.CronTaskFuncHandler(p.importPrices), core.CronTaskDefinitionOneTimeJob, nopArgsFactory, false)
	crn.RegisterTask(cronTaskUpdateSiaRenterPriceName, core.CronTaskFuncHandler(p.updatePrices), core.CronTaskDefinitionOneTimeJob, nopArgsFactory, false)
	return nil
}

func (p PriceTracker) ScheduleJobs(crn core.CronService) error {
	exists, rateJobItem := crn.JobExists(cronTaskRecordSiaRateName, nil)

	if !exists {
		err := crn.CreateJobScheduled(cronTaskRecordSiaRateName, nil)
		if err != nil {
			return err
		}
	} else {
		err := crn.CreateExistingJobScheduled(uuid.UUID(rateJobItem.UUID))
		if err != nil {
			return err
		}
	}

	err := crn.CreateJobIfNotExists(cronTaskImportSiaPriceHistoryName, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p PriceTracker) recordRate(_ core.CronTaskArgs, _ core.Context) error {
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

	if err = db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Create(&history)
	}); err != nil {
		p.logger.Error("failed to save price history", zap.Error(err))
	}

	err = p.cron.CreateJobIfNotExists(cronTaskUpdateSiaRenterPriceName, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p PriceTracker) updatePrices(_ any, _ core.Context) error {
	var averageRateStr string
	days := p.config.Config().Core.Storage.Sia.PriceHistoryDays

	var sql string
	if p.db.Dialector.Name() == "sqlite" {
		sql = `
        SELECT AVG(rate) as average_rate
        FROM sc_price_history
        WHERE created_at >= DATE('now', '-' || ? || ' days')
        `
	} else {
		sql = `
        SELECT AVG(rate) as average_rate
        FROM sc_price_history
        WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL ? DAY)
        `
	}

	err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Raw(sql, days).Scan(&averageRateStr)
	})

	if err != nil {
		p.logger.Error("failed to fetch average rate", zap.Error(err), zap.Uint64("days", days))
		return err
	}

	averageRate, err := decimal.NewFromString(averageRateStr)
	if err != nil {
		p.logger.Error("failed to parse average rate", zap.Error(err), zap.String("averageRateStr", averageRateStr))
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
	maxStoragePrice = ratDivide(maxStoragePrice, units.TB)
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

	err = p.cron.CreateJobIfNotExists(cronTaskUpdateSiaRenterPriceName, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p PriceTracker) importPrices(_ any, ctx core.Context) error {
	var existingDates []string // Change this to []string
	daysOfHistory := int(p.config.Config().Core.Storage.Sia.PriceHistoryDays)
	startDate := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -daysOfHistory)

	err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.SCPriceHistory{}).
			Where("created_at >= ?", startDate).
			Select("DATE(created_at) as date").
			Group("DATE(created_at)").
			Order("date ASC").
			Pluck("date", &existingDates)
	})
	if err != nil {
		p.logger.Error("failed to fetch existing historical dates", zap.Error(err))
		return err
	}

	existingDateMap := make(map[string]bool)
	for _, dateStr := range existingDates {
		existingDateMap[dateStr] = true
	}

	for i := 0; i < daysOfHistory; i++ {
		currentDate := startDate.AddDate(0, 0, i)
		dateKey := currentDate.Format("2006-01-02")
		if !existingDateMap[dateKey] {
			// Fetch and store data for currentDate as it's missing
			rates, err := p.api.GetHistoricalExchangeRate(currentDate)
			if err != nil {
				p.logger.Error("failed to fetch historical exchange rate", zap.Error(err), zap.Time("date", currentDate))
				continue // Skip to the next date if there's an error
			}

			// Assuming USD rates as an example
			rate, exists := rates[usdSymbol]
			if !exists {
				p.logger.Error("USD rate not found for date", zap.String("date", dateKey))
				continue // Skip to the next date
			}

			priceRecord := &models.SCPriceHistory{
				Rate:      rate,
				CreatedAt: currentDate,
			}

			if err = p.db.Transaction(func(tx *gorm.DB) error {
				return db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
					return db.FirstOrCreate(priceRecord)
				})
			}); err != nil {
				p.logger.Error("failed to create historical record for date", zap.String("date", dateKey), zap.Error(err))
				return err
			}
		}
	}

	err = p.cron.CreateJobIfNotExists(cronTaskUpdateSiaRenterPriceName, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p *PriceTracker) Init() error {
	p.cron.RegisterEntity(p)
	p.api = siasdk.NewSiaClient()

	return nil
}

func NewPriceTracker(ctx core.Context) *PriceTracker {
	return &PriceTracker{
		config: ctx.Config(),
		logger: ctx.Logger(),
		cron:   core.GetService[core.CronService](ctx, core.CRON_SERVICE),
		db:     ctx.DB(),
		renter: core.GetService[core.RenterService](ctx, core.RENTER_SERVICE),
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
