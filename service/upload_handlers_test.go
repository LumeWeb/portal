package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostDataHandler_CreateUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := &PostDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		// Test with nil data
		err := handler.CreateUploadData(context.Background(), db, 1, nil)
		assert.NoError(tb, err)

		// Test with some data
		err = handler.CreateUploadData(context.Background(), db, 1, "test data")
		assert.NoError(tb, err)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestPostDataHandler_GetUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := &PostDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		data, err := handler.GetUploadData(context.Background(), db, 1)
		assert.NoError(tb, err)
		assert.Nil(tb, data)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestPostDataHandler_UpdateUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := &PostDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		err := handler.UpdateUploadData(context.Background(), db, 1, nil)
		assert.NoError(tb, err)

		err = handler.UpdateUploadData(context.Background(), db, 1, "test data")
		assert.NoError(tb, err)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestPostDataHandler_DeleteUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := &PostDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		err := handler.DeleteUploadData(context.Background(), db, 1)
		assert.NoError(tb, err)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestPostDataHandler_QueryUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := &PostDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		query := map[string]interface{}{"test": "value"}
		tx := handler.QueryUploadData(context.Background(), db, query)
		assert.NotNil(tb, tx)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestPostDataHandler_CompleteUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := &PostDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		err := handler.CompleteUploadData(context.Background(), db, 1)
		assert.NoError(tb, err)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestPostDataHandler_GetUploadDataModel(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := &PostDataHandler{}
		model := handler.GetUploadDataModel()
		assert.Nil(tb, model)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSDataHandler_CreateUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := TUSDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a test TUSRequest
		tusRequest := &models.TUSRequest{
			TUSUploadID: "test_upload_id",
		}

		// Create upload data
		err := handler.CreateUploadData(context.Background(), db, 1, tusRequest)
		assert.NoError(tb, err)

		// Verify that the TUSRequest was created in the database
		var dbTUSRequest models.TUSRequest
		result := db.First(&dbTUSRequest, "request_id = ?", 1)
		require.NoError(tb, result.Error)
		assert.Equal(tb, "test_upload_id", dbTUSRequest.TUSUploadID)
		assert.Equal(tb, uint(1), dbTUSRequest.RequestID)

		// Test with empty TUSUploadID
		tusRequestEmpty := &models.TUSRequest{}
		err = handler.CreateUploadData(context.Background(), db, 2, tusRequestEmpty)
		assert.NoError(tb, err)

		// Verify that no TUSRequest was created in the database
		var dbTUSRequestEmpty models.TUSRequest
		result = db.First(&dbTUSRequestEmpty, "request_id = ?", 2)
		assert.Error(tb, result.Error)
		assert.True(tb, errors.Is(result.Error, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSDataHandler_GetUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := TUSDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a test TUSRequest in the database
		tusRequest := &models.TUSRequest{
			RequestID:   1,
			TUSUploadID: "test_upload_id",
		}
		err := db.Create(tusRequest).Error
		require.NoError(tb, err)

		// Get upload data
		data, err := handler.GetUploadData(context.Background(), db, 1)
		require.NoError(tb, err)
		assert.NotNil(tb, data)

		// Verify that the data is a TUSRequest
		retrievedTUSRequest, ok := data.(*models.TUSRequest)
		assert.True(tb, ok)
		assert.Equal(tb, "test_upload_id", retrievedTUSRequest.TUSUploadID)
		assert.Equal(tb, uint(1), retrievedTUSRequest.RequestID)

		// Test with non-existent request ID
		data, err = handler.GetUploadData(context.Background(), db, 999)
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))
		assert.Nil(tb, data)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSDataHandler_UpdateUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := TUSDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a test TUSRequest in the database
		tusRequest := &models.TUSRequest{
			RequestID:   1,
			TUSUploadID: "test_upload_id",
		}
		err := db.Create(tusRequest).Error
		require.NoError(tb, err)

		// Update the TUSRequest
		tusRequest.TUSUploadID = "new_upload_id"
		err = handler.UpdateUploadData(context.Background(), db, 1, tusRequest)
		require.NoError(tb, err)

		// Verify that the TUSRequest was updated in the database
		var dbTUSRequest models.TUSRequest
		result := db.First(&dbTUSRequest, "request_id = ?", 1)
		require.NoError(tb, result.Error)
		assert.Equal(tb, "new_upload_id", dbTUSRequest.TUSUploadID)
		assert.Equal(tb, uint(1), dbTUSRequest.RequestID)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSDataHandler_DeleteUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := TUSDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a test TUSRequest in the database
		tusRequest := &models.TUSRequest{
			RequestID:   1,
			TUSUploadID: "test_upload_id",
		}
		err := db.Create(tusRequest).Error
		require.NoError(tb, err)

		// Delete upload data
		err = handler.DeleteUploadData(context.Background(), db, 1)
		require.NoError(tb, err)

		// Verify that the TUSRequest was deleted from the database
		var dbTUSRequest models.TUSRequest
		result := db.First(&dbTUSRequest, "request_id = ?", 1)
		assert.Error(tb, result.Error)
		assert.True(tb, errors.Is(result.Error, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSDataHandler_QueryUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := TUSDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		// Define a query
		query := map[string]interface{}{"tus_upload_id": "test_upload_id"}

		// Query upload data
		tx := handler.QueryUploadData(context.Background(), db, query)
		assert.NotNil(tb, tx)

		// Verify that the query is applied correctly
		var tusRequest models.TUSRequest
		err := tx.Where(query).First(&tusRequest).Error
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSDataHandler_CompleteUploadData(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := TUSDataHandler{}
		db := ctx.DB()
		require.NotNil(tb, db)

		// Create a test TUSRequest in the database
		tusRequest := &models.TUSRequest{
			RequestID:   1,
			TUSUploadID: "test_upload_id",
		}
		err := db.Create(tusRequest).Error
		require.NoError(tb, err)

		// Complete upload data
		err = handler.CompleteUploadData(context.Background(), db, 1)
		require.NoError(tb, err)

		// Verify that the TUSRequest was deleted from the database
		var dbTUSRequest models.TUSRequest
		result := db.First(&dbTUSRequest, "request_id = ?", 1)
		assert.Error(tb, result.Error)
		assert.True(tb, errors.Is(result.Error, gorm.ErrRecordNotFound))

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}

func TestTUSDataHandler_GetUploadDataModel(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		handler := TUSDataHandler{}
		model := handler.GetUploadDataModel()
		assert.NotNil(tb, model)
		assert.IsType(tb, &models.TUSRequest{}, model)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, NewUserService))
}
