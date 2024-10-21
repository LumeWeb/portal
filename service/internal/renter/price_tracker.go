package renter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-queue/queue"
	core2 "github.com/golang-queue/queue/core"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"gorm.io/gorm/clause"
	"math/big"
	"sync"
	"sync/atomic"
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
	ctx    core.Context
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
	var averageRateStr sql.NullString
	days := p.config.Config().Core.Storage.Sia.PriceHistoryDays

	var _sql string
	if p.db.Dialector.Name() == "sqlite" {
		_sql = `
        SELECT COALESCE(AVG(rate), '0') as average_rate
        FROM sc_price_history
        WHERE created_at >= DATE('now', '-' || ? || ' days')
        `
	} else {
		_sql = `
        SELECT COALESCE(AVG(rate), '0') as average_rate
        FROM sc_price_history
        WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL ? DAY)
        `
	}

	err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Raw(_sql, days).Scan(&averageRateStr)
	})

	if err != nil {
		p.logger.Error("failed to fetch average rate", zap.Error(err), zap.Uint64("days", days))
		return err
	}

	if !averageRateStr.Valid || averageRateStr.String == "" {
		p.logger.Error("average rate is NULL or empty")
		return errors.New("average rate is NULL or empty")
	}

	averageRate, err := decimal.NewFromString(averageRateStr.String)
	if err != nil {
		p.logger.Error("failed to parse average rate", zap.Error(err), zap.String("averageRateStr", averageRateStr.String))
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
	var existingDates []string
	daysOfHistory := int(p.config.Config().Core.Storage.Sia.PriceHistoryDays)
	startDate := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -daysOfHistory)

	if err := db.RetryableTransaction(p.ctx, p.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.SCPriceHistory{}).
			Where("created_at >= ?", startDate).
			Select("DATE(created_at) as date").
			Group("DATE(created_at)").
			Order("date ASC").
			Pluck("date", &existingDates)
	}); err != nil {
		p.logger.Error("failed to fetch existing historical dates", zap.Error(err))
		return err
	}

	existingDateMap := make(map[string]bool)
	for _, dateStr := range existingDates {
		existingDateMap[dateStr] = true
	}

	var wg sync.WaitGroup
	var errorOccurred atomic.Bool

	var q *queue.Queue
	queueOptions := []queue.Option{
		queue.WithWorker(queue.NewRing(
			queue.WithFn(func(ctx context.Context, msg core2.QueuedMessage) error {
				var job message
				if err := json.Unmarshal(msg.Bytes(), &job); err != nil {
					return err
				}

				dateKey := job.Date.Format("2006-01-02")
				currentDate := job.Date

				rates, err := p.api.GetHistoricalExchangeRate(currentDate)
				if err != nil {
					p.logger.Error("failed to fetch historical exchange rate", zap.Error(err), zap.Time("date", currentDate))
					return nil
				}

				rate, exists := rates[usdSymbol]
				if !exists {
					p.logger.Error("USD rate not found for date", zap.String("date", dateKey))
					return nil
				}

				priceRecord := &models.SCPriceHistory{
					Rate:      rate,
					CreatedAt: currentDate,
				}

				if err := db.RetryableTransaction(p.ctx, p.db, func(tx *gorm.DB) *gorm.DB {
					return tx.Clauses(clause.OnConflict{
						Columns:   []clause.Column{{Name: "created_at"}},
						DoUpdates: clause.AssignmentColumns([]string{"rate"}),
					}).Create(&priceRecord)
				}); err != nil {
					p.logger.Error("failed to upsert historical records", zap.Error(err))
					errorOccurred.Store(true)
				}

				return nil
			}),
			queue.WithWorkerCount(p.config.Config().Core.Storage.Sia.PriceFetchWorkers),
			queue.WithLogger(newZapLogAdapter(p.logger)),
		)),
	}

	// Create a closure that captures the queue variable
	var afterFn func()
	afterFn = func() {
		wg.Done()
		if q.BusyWorkers() == 0 {
			q.Release()
		}
	}

	// Add the afterFn option
	queueOptions = append(queueOptions, queue.WithAfterFn(afterFn))

	// Create the queue
	q, err := queue.NewQueue(queueOptions...)
	if err != nil {
		p.logger.Error("Failed to create queue", zap.Error(err))
		return err
	}

	for i := 0; i < daysOfHistory; i++ {
		currentDate := startDate.AddDate(0, 0, i)
		dateKey := currentDate.Format("2006-01-02")
		if !existingDateMap[dateKey] {
			wg.Add(1)
			err := q.Queue(message{Date: currentDate})
			if err != nil {
				p.logger.Error("Failed to queue job", zap.Error(err))
				errorOccurred.Store(true)
				wg.Done()
			}
		}
	}

	// Start the queue
	q.Start()

	// Wait for all jobs to be processed
	wg.Wait()

	if errorOccurred.Load() {
		p.logger.Error("errors occurred during price import")
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
		ctx:    ctx,
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

type zapAdapter struct {
	logger *core.Logger
}

func newZapLogAdapter(logger *core.Logger) *zapAdapter {
	return &zapAdapter{logger: logger}
}

func (za *zapAdapter) Infof(format string, args ...interface{}) {
	za.logger.Info(fmt.Sprintf(format, args...))
}

func (za *zapAdapter) Errorf(format string, args ...interface{}) {
	za.logger.Error(fmt.Sprintf(format, args...))
}

func (za *zapAdapter) Fatalf(format string, args ...interface{}) {
	za.logger.Fatal(fmt.Sprintf(format, args...))
}

func (za *zapAdapter) Info(args ...interface{}) {
	za.logger.Info(fmt.Sprint(args...))
}

func (za *zapAdapter) Error(args ...interface{}) {
	za.logger.Error(fmt.Sprint(args...))
}

func (za *zapAdapter) Fatal(args ...interface{}) {
	za.logger.Fatal(fmt.Sprint(args...))
}

var _ core2.QueuedMessage = (*message)(nil)

type message struct {
	Date time.Time
}

func (m message) Bytes() []byte {
	bytes, _ := json.Marshal(m)
	return bytes
}
