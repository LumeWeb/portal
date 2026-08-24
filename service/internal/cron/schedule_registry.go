package cron

import (
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"go.lumeweb.com/portal/core"
	"strings"
	"sync"
	"time"
)

const (
	TimeFormatHHMMSS  = "15:04:05"
	TimeFormatRFC3339 = time.RFC3339
)

type DefaultScheduleRegistry struct {
	factories map[core.CronScheduleType]core.ScheduleFactoryFunc
	mu        *sync.RWMutex
}

var _ core.CronScheduleRegistry = (*DefaultScheduleRegistry)(nil)

func NewScheduleRegistry() *DefaultScheduleRegistry {
	r := &DefaultScheduleRegistry{
		factories: make(map[core.CronScheduleType]core.ScheduleFactoryFunc),
		mu:        &sync.RWMutex{},
	}
	r.registerBuiltinTypes()
	r.registerInternalSchedules()
	return r
}

func (r *DefaultScheduleRegistry) registerInternalSchedules() {
	r.Register(DeadJobCheckJobType, func(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
		return gocron.DurationJob(time.Duration(def.Interval) * time.Minute), nil
	})

	r.Register(CleanupJobType, func(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
		return gocron.DailyJob(def.Interval, gocron.NewAtTimes(gocron.NewAtTime(0, 0, 0))), nil
	})
}

// Register adds a schedule factory for a given type
func (r *DefaultScheduleRegistry) Register(schedType core.CronScheduleType, factory core.ScheduleFactoryFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[schedType] = factory
}

// Create converts a CronScheduleDefinition to a gocron.JobDefinition
func (r *DefaultScheduleRegistry) Create(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.factories[def.Type]
	if !ok {
		return nil, fmt.Errorf("unknown schedule type: %s", def.Type)
	}
	return factory(def)
}

// GetRegisteredTypes returns all registered schedule types
func (r *DefaultScheduleRegistry) GetRegisteredTypes() []core.CronScheduleType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]core.CronScheduleType, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// Validate checks if a schedule definition is valid
func (r *DefaultScheduleRegistry) Validate(def core.CronScheduleDefinition) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.factories[def.Type]
	if !ok {
		return fmt.Errorf("unknown schedule type: %s", def.Type)
	}
	return nil
}

func (r *DefaultScheduleRegistry) createDailyJob(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	interval := uint(1)
	if def.Interval > 0 {
		interval = def.Interval
	}

	if def.AtTime.IsZero() {
		now := time.Now()
		return gocron.DailyJob(interval, gocron.NewAtTimes(gocron.NewAtTime(uint(now.Hour()), uint(now.Minute()), uint(now.Second())))), nil
	}

	return gocron.DailyJob(interval, gocron.NewAtTimes(gocron.NewAtTime(uint(def.AtTime.Hour()), uint(def.AtTime.Minute()), uint(def.AtTime.Second())))), nil
}

func (r *DefaultScheduleRegistry) createWeeklyJob(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	interval := uint(1)
	if def.Interval > 0 {
		interval = def.Interval
	}

	// Default to Monday if DayOfWeek not specified
	weekday := time.Monday
	if def.DayOfWeek != "" {
		var err error
		weekday, err = parseWeekday(def.DayOfWeek)
		if err != nil {
			return nil, fmt.Errorf("invalid day of week: %w", err)
		}
	}

	if def.AtTime.IsZero() {
		now := time.Now()
		return gocron.WeeklyJob(interval, gocron.NewWeekdays(weekday), gocron.NewAtTimes(gocron.NewAtTime(uint(now.Hour()), uint(now.Minute()), uint(now.Second())))), nil
	}

	return gocron.WeeklyJob(interval, gocron.NewWeekdays(weekday), gocron.NewAtTimes(gocron.NewAtTime(uint(def.AtTime.Hour()), uint(def.AtTime.Minute()), uint(def.AtTime.Second())))), nil
}

// parseWeekday converts a weekday string to time.Weekday
func parseWeekday(day string) (time.Weekday, error) {
	switch strings.ToLower(day) {
	case "monday", "mon":
		return time.Monday, nil
	case "tuesday", "tue":
		return time.Tuesday, nil
	case "wednesday", "wed":
		return time.Wednesday, nil
	case "thursday", "thu":
		return time.Thursday, nil
	case "friday", "fri":
		return time.Friday, nil
	case "saturday", "sat":
		return time.Saturday, nil
	case "sunday", "sun":
		return time.Sunday, nil
	default:
		return time.Monday, fmt.Errorf("invalid weekday: %s", day)
	}
}

func (r *DefaultScheduleRegistry) createMonthlyJob(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	interval := uint(1)
	if def.Interval > 0 {
		interval = def.Interval
	}

	day := 1
	if def.DayOfMonth > 0 {
		day = def.DayOfMonth
	}

	if def.AtTime.IsZero() {
		now := time.Now()
		return gocron.MonthlyJob(interval, gocron.NewDaysOfTheMonth(day), gocron.NewAtTimes(gocron.NewAtTime(uint(now.Hour()), uint(now.Minute()), uint(now.Second())))), nil
	}

	return gocron.MonthlyJob(interval, gocron.NewDaysOfTheMonth(day), gocron.NewAtTimes(gocron.NewAtTime(uint(def.AtTime.Hour()), uint(def.AtTime.Minute()), uint(def.AtTime.Second())))), nil
}

func (r *DefaultScheduleRegistry) createCronJob(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	if def.CronExpression == "" {
		return nil, fmt.Errorf("cron expression is required")
	}
	return gocron.CronJob(def.CronExpression, false), nil
}

func (r *DefaultScheduleRegistry) createHourlyJob(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	interval := uint(1)
	if def.Interval > 0 {
		interval = def.Interval
	}
	return gocron.DurationJob(time.Duration(interval) * time.Hour), nil
}

// createDurationJob creates a job that runs every Interval minutes.
func (r *DefaultScheduleRegistry) createDurationJob(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	interval := uint(30)
	if def.Interval > 0 {
		interval = def.Interval
	}
	return gocron.DurationJob(time.Duration(interval) * time.Minute), nil
}

func (r *DefaultScheduleRegistry) createOnceJob(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
	if def.AtTime.IsZero() {
		def.AtTime = time.Now().Add(10 * time.Second)
	}

	return gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(def.AtTime)), nil
}

func (r *DefaultScheduleRegistry) registerBuiltinTypes() {
	// Register built-in schedule types
	r.Register(core.CronScheduleTypeDaily, r.createDailyJob)
	r.Register(core.CronScheduleTypeWeekly, r.createWeeklyJob)
	r.Register(core.CronScheduleTypeMonthly, r.createMonthlyJob)
	r.Register(core.CronScheduleTypeHourly, r.createHourlyJob)
	r.Register(core.CronScheduleTypeCron, r.createCronJob)
	r.Register(core.CronScheduleTypeOnce, r.createOnceJob)
	r.Register(core.CronScheduleTypeDuration, r.createDurationJob)
}
