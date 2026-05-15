package service_tests

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service"
	"gorm.io/datatypes"
)

func TestJobCreator_CreateFromDB_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		testJobID := uuid.New()
		testJobType := "core.cron.integration-test-job"
		testArgs := `{"key":"value"}`

		db := ctx.DB()
		err := db.Create(&models.CronJob{
			UUID:    types.FromUUID(testJobID),
			JobType: testJobType,
			Args:    datatypes.JSON(testArgs),
		}).Error
		require.NoError(tb, err)

		mockFactory := mocks.NewMockCronJobFactory(tb)
		mockJob := mocks.NewMockCronJob(tb)
		mockFactory.EXPECT().CreateJob(mock.Anything, testJobType).Return(mockJob, nil)
		mockJob.EXPECT().SetArgs(map[string]interface{}{"key": "value"}).Return()

		creator := service.NewJobCreator(db, mockFactory, ctx.Logger())

		job, err := creator.CreateFromDB(context.Background(), testJobID)

		require.NoError(tb, err)
		assert.Equal(t, mockJob, job)
	})
}

func TestJobCreator_CreateFromDB_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		nonExistentID := uuid.New()

		mockFactory := mocks.NewMockCronJobFactory(tb)

		db := ctx.DB()
		creator := service.NewJobCreator(db, mockFactory, ctx.Logger())

		job, err := creator.CreateFromDB(context.Background(), nonExistentID)

		require.Error(tb, err)
		assert.Nil(t, job)
		assert.Contains(t, err.Error(), "failed to find job in database")
	})
}
