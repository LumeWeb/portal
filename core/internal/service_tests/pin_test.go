package service_tests

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	dbMocks "go.lumeweb.com/portal/db/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinService_RegisterPinModel(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		protocol := "test_protocol"
		model := dbMocks.NewMockPinDataModel(t)

		svc.RegisterPinModel(protocol, model)

		retrieved, exists := svc.GetPinModel(protocol)
		assert.True(tb, exists)
		assert.Equal(tb, model, retrieved)
	}, coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_CreatePin(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		upload := &models.Upload{
			Protocol: "test",
			Hash:     []byte("testhash"),
		}
		err := ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		pin := &models.Pin{
			UserID:   1,
			UploadID: upload.ID,
		}

		protocol := "test"
		model := dbMocks.NewMockPinDataModel(t)
		svc.RegisterPinModel(protocol, model)

		// Create test protocol that implements ProtocolPinHandler
		testProto := coreTesting.NewMockProtocol(t, protocol)
		core.RegisterProtocol(protocol, testProto)

		createdPin, err := svc.CreatePin(context.Background(), pin, nil)
		require.NoError(tb, err)
		assert.NotNil(tb, createdPin)
		assert.Equal(tb, pin.UserID, createdPin.UserID)
		assert.Equal(tb, pin.UploadID, createdPin.UploadID)

		var dbPin models.Pin
		err = ctx.DB().First(&dbPin, createdPin.ID).Error
		require.NoError(tb, err)
	}, coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_GetPinsByUploadID(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		upload := &models.Upload{
			Protocol: "test",
			Hash:     []byte("testhash2"),
		}
		err := ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		pin1 := &models.Pin{UserID: 1, UploadID: upload.ID}
		pin2 := &models.Pin{UserID: 2, UploadID: upload.ID}
		err = ctx.DB().Create([]*models.Pin{pin1, pin2}).Error
		require.NoError(tb, err)

		pins, err := svc.GetPinsByUploadID(context.Background(), upload.ID)
		require.NoError(tb, err)
		assert.Len(tb, pins, 2)
	}, coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_DeletePinByHash(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PinService](ctx, core.PIN_SERVICE)

		protocol := "test"
		model := dbMocks.NewMockPinDataModel(t)
		svc.RegisterPinModel(protocol, model)
		// Create test protocol that implements ProtocolPinHandler
		testProto := coreTesting.NewMockProtocol(t, protocol)
		core.RegisterProtocol(protocol, testProto)

		upload := &models.Upload{
			Protocol: "test",
			Hash:     []byte("testhash3"),
		}
		err := ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		pin := &models.Pin{UserID: 1, UploadID: upload.ID}
		err = ctx.DB().Create(pin).Error
		require.NoError(tb, err)

		mockHash := coreMocks.NewMockStorageHash(t)
		mockHash.On("Multihash").Return(upload.Hash)

		err = svc.DeletePinByHash(context.Background(), mockHash, pin.UserID)
		require.NoError(tb, err)

		var dbPin models.Pin
		err = ctx.DB().First(&dbPin, pin.ID).Error
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))
	}, coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_UploadPinnedGlobal(t *testing.T) {
	t.Skip("stringer mock bug")
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		uploadSvc := core.GetService[*coreMocks.MockUploadService](ctx, core.UPLOAD_SERVICE)

		upload := &models.Upload{
			Protocol: "test",
			Hash:     []byte("testhash4"),
		}
		err := ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		pin := &models.Pin{UserID: 1, UploadID: upload.ID}
		err = ctx.DB().Create(pin).Error
		require.NoError(tb, err)

		mockHash := coreMocks.NewMockStorageHash(t)
		mockHash.On("String").Return(upload.Hash.String())
		mockHash.EXPECT().Multihash().Return(upload.Hash)

		uploadSvc.EXPECT().GetUpload(mock.Anything, mockHash).Return(upload, nil)

		pinned, err := svc.UploadPinnedGlobal(context.Background(), mockHash)
		require.NoError(tb, err)
		assert.True(tb, pinned)

		mockHash2 := coreMocks.NewMockStorageHash(t)
		mockHash2.On("String").Return("")
		mockHash2.EXPECT().Multihash().Return([]byte("nonexistent"))

		pinned, err = svc.UploadPinnedGlobal(context.Background(), mockHash2)
		require.NoError(tb, err)
		assert.False(tb, pinned)
	}, coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_UploadPinnedByUser(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PinService](ctx, core.PIN_SERVICE)

		upload := &models.Upload{
			Protocol: "test",
			Hash:     []byte("testhash5"),
		}
		err := ctx.DB().Create(upload).Error
		require.NoError(tb, err)

		userID := uint(1)
		pin := &models.Pin{UserID: userID, UploadID: upload.ID}
		err = ctx.DB().Create(pin).Error
		require.NoError(tb, err)

		mockHash := coreMocks.NewMockStorageHash(t)
		mockHash.On("Multihash").Return(upload.Hash).Maybe()
		mockHash.On("String").Return("testhash5").Maybe()

		// Setup mock expectation for GetUpload
		uploadService := coreTesting.GetMockUploadService(ctx)
		uploadService.EXPECT().GetUpload(mock.Anything, mock.Anything).Return(upload, nil).Maybe()

		pinned, err := svc.UploadPinnedByUser(context.Background(), mockHash, userID)
		require.NoError(tb, err)
		assert.True(tb, pinned)

		pinned, err = svc.UploadPinnedByUser(context.Background(), mockHash, 2)
		require.NoError(tb, err)
		assert.False(tb, pinned)
	}, coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}

func TestPinService_GetPinStats(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
		require.NotNil(tb, svc)

		// Empty DB — returns empty slice
		stats, err := svc.GetPinStats(context.Background())
		require.NoError(tb, err)
		assert.Empty(tb, stats)

		// Create uploads across two protocols
		upload1 := &models.Upload{Protocol: "ipfs", Hash: []byte("hash1")}
		upload2 := &models.Upload{Protocol: "ipfs", Hash: []byte("hash2")}
		upload3 := &models.Upload{Protocol: "sia", Hash: []byte("hash3")}
		for _, u := range []*models.Upload{upload1, upload2, upload3} {
			require.NoError(tb, ctx.DB().Create(u).Error)
		}

		// Create pins: 3 for ipfs (2 on upload1, 1 on upload2), 1 for sia
		pins := []*models.Pin{
			{UserID: 1, UploadID: upload1.ID},
			{UserID: 2, UploadID: upload1.ID},
			{UserID: 1, UploadID: upload2.ID},
			{UserID: 1, UploadID: upload3.ID},
		}
		for _, p := range pins {
			require.NoError(tb, ctx.DB().Create(p).Error)
		}

		stats, err = svc.GetPinStats(context.Background())
		require.NoError(tb, err)
		require.Len(tb, stats, 2)

		byProtocol := make(map[string]core.ProtocolPinStat)
		for _, s := range stats {
			byProtocol[s.Protocol] = s
		}

		// IPFS: 3 pins across 2 uploads
		ipfsStat, ok := byProtocol["ipfs"]
		assert.True(tb, ok)
		assert.Equal(tb, uint64(3), ipfsStat.TotalPins)

		// Sia: 1 pin
		siaStat, ok := byProtocol["sia"]
		assert.True(tb, ok)
		assert.Equal(tb, uint64(1), siaStat.TotalPins)

		// Soft-delete the sia upload and verify its pin is excluded
		require.NoError(tb, ctx.DB().Delete(upload3).Error)
		stats, err = svc.GetPinStats(context.Background())
		require.NoError(tb, err)
		require.Len(tb, stats, 1)

		byProtocol = make(map[string]core.ProtocolPinStat)
		for _, s := range stats {
			byProtocol[s.Protocol] = s
		}
		_, siaOk := byProtocol["sia"]
		assert.False(tb, siaOk)
		ipfsStat = byProtocol["ipfs"]
		assert.Equal(tb, uint64(3), ipfsStat.TotalPins)

	}, coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService))
}
