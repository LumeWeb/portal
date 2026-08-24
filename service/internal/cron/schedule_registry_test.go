package cron

import (
	"github.com/go-co-op/gocron/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"testing"
	"time"
)

func parseTimeOrPanic(timeStr string, layout string) time.Time {
	t, err := time.Parse(layout, timeStr)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDefaultScheduleRegistry_Register(t *testing.T) {
	registry := NewScheduleRegistry()
	scheduleType := core.CronScheduleType("test_type")
	factory := func(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
		return nil, nil
	}

	registry.Register(scheduleType, factory)

	registeredTypes := registry.GetRegisteredTypes()
	assert.Contains(t, registeredTypes, scheduleType)
}

func TestDefaultScheduleRegistry_Create(t *testing.T) {
	registry := NewScheduleRegistry()
	scheduleType := core.CronScheduleType("test_type")
	factory := func(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
		return gocron.DurationJob(time.Second), nil
	}
	registry.Register(scheduleType, factory)

	def := core.CronScheduleDefinition{
		Type: scheduleType,
	}

	jobDefinition, err := registry.Create(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}

func TestDefaultScheduleRegistry_Create_UnknownType(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type: "unknown_type",
	}

	_, err := registry.Create(def)
	assert.Error(t, err)
	assert.Equal(t, "unknown schedule type: unknown_type", err.Error())
}

func TestDefaultScheduleRegistry_GetRegisteredTypes(t *testing.T) {
	registry := NewScheduleRegistry()
	types := registry.GetRegisteredTypes()

	assert.Contains(t, types, core.CronScheduleTypeDaily)
	assert.Contains(t, types, core.CronScheduleTypeWeekly)
	assert.Contains(t, types, core.CronScheduleTypeMonthly)
	assert.Contains(t, types, core.CronScheduleTypeCron)
	assert.Contains(t, types, core.CronScheduleTypeOnce)
	assert.Contains(t, types, core.CronScheduleTypeDuration)
}

func TestDefaultScheduleRegistry_Validate(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type: core.CronScheduleTypeDaily,
	}

	err := registry.Validate(def)
	assert.NoError(t, err)
}

func TestDefaultScheduleRegistry_Validate_UnknownType(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type: "unknown_type",
	}

	err := registry.Validate(def)
	assert.Error(t, err)
	assert.Equal(t, "unknown schedule type: unknown_type", err.Error())
}

func TestDefaultScheduleRegistry_createDailyJob(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type:     core.CronScheduleTypeDaily,
		Interval: 2,
		AtTime:   parseTimeOrPanic("10:15:30", TimeFormatHHMMSS),
	}

	jobDefinition, err := registry.createDailyJob(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}

func TestDefaultScheduleRegistry_createWeeklyJob(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type:     core.CronScheduleTypeWeekly,
		Interval: 2,
		AtTime:   parseTimeOrPanic("10:15:30", TimeFormatHHMMSS),
	}

	jobDefinition, err := registry.createWeeklyJob(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}

func TestDefaultScheduleRegistry_createMonthlyJob(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type:     core.CronScheduleTypeMonthly,
		Interval: 2,
		AtTime:   parseTimeOrPanic("10:15:30", TimeFormatHHMMSS),
	}

	jobDefinition, err := registry.createMonthlyJob(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}

func TestDefaultScheduleRegistry_createCronJob(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type:           core.CronScheduleTypeCron,
		CronExpression: "0 0 * * *",
	}

	jobDefinition, err := registry.createCronJob(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}

func TestDefaultScheduleRegistry_createCronJob_MissingExpression(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type: core.CronScheduleTypeCron,
	}

	_, err := registry.createCronJob(def)
	assert.Error(t, err)
}

func TestDefaultScheduleRegistry_createOnceJob(t *testing.T) {
	registry := NewScheduleRegistry()
	testTime, err := time.Parse(time.RFC3339, "2024-01-02T15:04:05Z")
	require.NoError(t, err)

	def := core.CronScheduleDefinition{
		Type:   core.CronScheduleTypeOnce,
		AtTime: testTime,
	}

	jobDefinition, err := registry.createOnceJob(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}

func TestDefaultScheduleRegistry_createOnceJob_MissingTime(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type: core.CronScheduleTypeOnce,
	}

	jobDefinition, err := registry.createOnceJob(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}

func TestDefaultScheduleRegistry_createDurationJob(t *testing.T) {
	registry := NewScheduleRegistry()
	def := core.CronScheduleDefinition{
		Type:     core.CronScheduleTypeDuration,
		Interval: 30,
	}

	jobDefinition, err := registry.createDurationJob(def)
	assert.NoError(t, err)
	assert.NotNil(t, jobDefinition)
}
