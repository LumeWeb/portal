package service_tests

import (
	"context"
	"testing"

	"github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinService_CreatePin_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		pinService := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		require.NotNil(tb, pinService)

		protocol := "test"
		testProto := coreTesting.NewMockProtocol(t, protocol)
		pinService.RegisterPinModel(protocol, testProto.PinHandler().GetProtocolPinModel())

		// Create test protocol with pin handler

		core.RegisterProtocol(protocol, testProto)

		// 1. Create a test upload
		// Create unique SHA-256 multihash for each test case
		testData := []byte("test_data_for_hashing_" + protocol + "_" + tb.Name())
		mh, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		upload := &models.Upload{
			Protocol: protocol,
			Hash:     mh,
		}
		err = ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		// 2. Create a pin associated with the upload
		pin := &models.Pin{
			UserID:   1,
			UploadID: upload.ID,
		}

		createdPin, err := pinService.CreatePin(context.Background(), pin, nil)
		require.NoError(tb, err)
		assert.NotNil(tb, createdPin)
		assert.Equal(tb, pin.UserID, createdPin.UserID)
		assert.Equal(tb, pin.UploadID, createdPin.UploadID)

		// 3. Verify the pin exists in the database
		var dbPin models.Pin
		err = ctx.DB().First(&dbPin, createdPin.ID).Error
		require.NoError(tb, err)
		assert.Equal(tb, pin.UserID, dbPin.UserID)
		assert.Equal(tb, pin.UploadID, dbPin.UploadID)

	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService),
		coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_GetPinsByUploadID_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		pinService := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		require.NotNil(tb, pinService)

		protocol := "test"
		testProto := coreTesting.NewMockProtocol(t, protocol)
		pinService.RegisterPinModel(protocol, testProto.PinHandler().GetProtocolPinModel())

		// Create test protocol that implements ProtocolPinHandler
		core.RegisterProtocol(protocol, testProto)

		// 1. Create a test upload
		// Create SHA-256 multihash for test data
		testData := []byte("test_data_for_hashing")
		mh, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		upload := &models.Upload{
			Protocol: protocol,
			Hash:     mh,
		}
		err = ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		// 2. Create multiple pins associated with the upload
		pin1 := &models.Pin{UserID: 1, UploadID: upload.ID}
		pin2 := &models.Pin{UserID: 2, UploadID: upload.ID}
		err = ctx.DB().Create([]*models.Pin{pin1, pin2}).Error
		require.NoError(tb, err)

		// 3. Retrieve pins by upload ID
		pins, err := pinService.GetPinsByUploadID(context.Background(), upload.ID)
		require.NoError(tb, err)
		assert.Len(tb, pins, 2)

		// 4. Verify the retrieved pins
		assert.Equal(tb, uint(1), pins[0].UserID)
		assert.Equal(tb, upload.ID, pins[0].UploadID)
		assert.Equal(tb, uint(2), pins[1].UserID)
		assert.Equal(tb, upload.ID, pins[1].UploadID)

	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService),
		coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_DeletePinByHash_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		pinService := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		require.NotNil(tb, pinService)

		protocol := "test"
		testProto := coreTesting.NewMockProtocol(t, protocol)
		pinService.RegisterPinModel(protocol, testProto.PinHandler().GetProtocolPinModel())

		// Create test protocol that implements ProtocolPinHandler
		core.RegisterProtocol(protocol, testProto)

		// 1. Create a test upload
		// Create SHA-256 multihash for test data
		testData := []byte("test_data_for_hashing")
		mh, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		upload := &models.Upload{
			Protocol: protocol,
			Hash:     mh,
		}
		err = ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		// 2. Create a pin associated with the upload
		pin := &models.Pin{UserID: 1, UploadID: upload.ID}
		err = ctx.DB().Create(pin).Error
		require.NoError(tb, err)

		// 3. Delete the pin by hash
		err = pinService.DeletePinByHash(nil, &testStorageHash{mh: upload.Hash, hash: testData}, pin.UserID)
		require.NoError(tb, err)

		// 4. Verify the pin is deleted
		var dbPin models.Pin
		err = ctx.DB().First(&dbPin, pin.ID).Error
		assert.Error(tb, err)
		assert.ErrorIs(tb, err, gorm.ErrRecordNotFound)

	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService),
		coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_UploadPinnedGlobal_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		pinService := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		require.NotNil(tb, pinService)

		protocol := "test"
		testProto := coreTesting.NewMockProtocol(t, protocol)
		pinService.RegisterPinModel(protocol, testProto.PinHandler().GetProtocolPinModel())

		// Create test protocol that implements ProtocolPinHandler
		core.RegisterProtocol(protocol, testProto)

		// 1. Create a test upload
		// Create SHA-256 multihash for test data
		testData := []byte("test_data_for_hashing")
		mh, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		upload := &models.Upload{
			Protocol: protocol,
			Hash:     mh,
		}
		err = ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		// 2. Create a pin associated with the upload
		pin := &models.Pin{UserID: 1, UploadID: upload.ID}
		err = ctx.DB().Create(pin).Error
		require.NoError(tb, err)

		// 3. Check if the upload is pinned globally
		pinned, err := pinService.UploadPinnedGlobal(nil, &testStorageHash{mh: upload.Hash, hash: testData})
		require.NoError(tb, err)
		assert.True(tb, pinned)

		// 4. Check if a non-existent upload is pinned globally
		pinned, err = pinService.UploadPinnedGlobal(nil, &testStorageHash{hash: []byte("nonexistent_hash")})
		require.NoError(tb, err)
		assert.False(tb, pinned)

	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService),
		coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_UploadPinnedByUser_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		pinService := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		require.NotNil(tb, pinService)

		protocol := "test"
		testProto := coreTesting.NewMockProtocol(t, protocol)
		pinService.RegisterPinModel(protocol, testProto.PinHandler().GetProtocolPinModel())

		// Create test protocol that implements ProtocolPinHandler
		core.RegisterProtocol(protocol, testProto)

		// 1. Create a test upload
		// Create SHA-256 multihash for test data
		testData := []byte("test_data_for_hashing")
		mh, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		if err != nil {
			tb.Fatal(err)
		}

		upload := &models.Upload{
			Protocol: protocol,
			Hash:     mh,
		}
		err = ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		// 2. Create a pin associated with the upload and a specific user
		userID := uint(1)
		pin := &models.Pin{UserID: userID, UploadID: upload.ID}
		err = ctx.DB().Create(pin).Error
		require.NoError(tb, err)

		// 3. Check if the upload is pinned by the user
		pinned, err := pinService.UploadPinnedByUser(nil, &testStorageHash{mh: upload.Hash, hash: testData}, userID)
		require.NoError(tb, err)
		assert.True(tb, pinned)

		// 4. Check if the upload is pinned by a different user
		pinned, err = pinService.UploadPinnedByUser(nil, &testStorageHash{mh: upload.Hash, hash: testData}, 2)
		require.NoError(tb, err)
		assert.False(tb, pinned)

		// 5. Check if a non-existent upload is pinned by the user
		pinned, err = pinService.UploadPinnedByUser(nil, &testStorageHash{hash: []byte("nonexistent_hash")}, userID)
		require.NoError(tb, err)
		assert.False(tb, pinned)

	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService),
		coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}
