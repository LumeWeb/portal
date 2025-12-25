package cron

import (
	"go.lumeweb.com/portal/db/types"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
)

func TestJobCreator_CreateFromDB_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Setup test data
		testJobID := uuid.New()
		testJobType := "core.cron.integration-test-job"
		testArgs := `{"key":"value"}`

		// Create test record in DB
		db := ctx.DB()
		err := db.Create(&models.CronJob{
			UUID:    types.FromUUID(testJobID),
			JobType: testJobType,
			Args:    testArgs,
		}).Error
		require.NoError(tb, err)

		// Create mock factory and job using proper constructors
		mockFactory := mocks.NewMockCronJobFactory(tb)
		mockJob := mocks.NewMockCronJob(tb)
		mockFactory.On("CreateJob", "core.cron.integration-test-job").Return(mockJob, nil)
		mockJob.On("Args").Return(&map[string]interface{}{"key": "value"})
		mockJob.On("SetArgs", map[string]interface{}{"key": "value"}).Return()

		// Create JobCreator with real DB and context logger
		creator := NewJobCreator(db, mockFactory, ctx.Logger())

		// Execute
		job, err := creator.CreateFromDB(nil, testJobID)

		// Verify
		require.NoError(tb, err)
		assert.Equal(t, mockJob, job)
		mockFactory.AssertExpectations(tb)
	})
}

func TestJobCreator_CreateFromDB_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Setup non-existent job ID
		nonExistentID := uuid.New()

		// Create mock factory (not expecting any calls)
		mockFactory := mocks.NewMockCronJobFactory(tb)

		// Create JobCreator with real DB
		db := ctx.DB()
		creator := NewJobCreator(db, mockFactory, ctx.Logger())

		// Execute
		job, err := creator.CreateFromDB(nil, nonExistentID)

		// Verify
		require.Error(tb, err)
		assert.Nil(t, job)
		assert.Contains(t, err.Error(), "failed to find job in database")
		mockFactory.AssertExpectations(tb)
	})
}
