package service_tests

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.sia.tech/core/types"
	sdk "go.sia.tech/siastorage"

	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	svcTesting "go.lumeweb.com/portal/service/testing"
	proto4 "go.sia.tech/core/rhp/v4"
	"gorm.io/gorm"
)

// withRenterService returns a TestContextBuilderOption that registers a manually
// constructed RenterService (without the packing loop) in the test context.
// BaseComponent is auto-injected by core's ContextWithStartupComponent during boot.
func withRenterService(renter *service.RenterService) coreTesting.TestContextBuilderOption {
	return coreTesting.WrapCoreOption(core.ContextWithStartupComponent(renter))
}

// withRenterServiceAndMocks is a convenience wrapper that creates and registers
// a RenterService with mock dependencies.
func withRenterServiceAndMocks(slabSize int64) (*service.RenterService, *svcTesting.MockRenterSDK, *svcTesting.MockStagingBackend, coreTesting.TestContextBuilderOption) {
	renter, mockSDK, mockStaging := svcTesting.SetupRenterService(slabSize)
	opt := withRenterService(renter)
	return renter, mockSDK, mockStaging, opt
}

// ============================================================================
// Public API tests — exercise service.RenterService via public methods only.
// Mocks live in service/testing/renter_mocks.go.
// ============================================================================

func TestRenterService_UploadObject_Direct(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("a"), int(slabSize*2))
		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "testfile.dat", []byte("hash123"))
		require.NoError(tb, err)

		assert.Len(tb, mockSDK.PinnedObjs, 1)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "testfile.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusUploaded, siaObj.Status)
		assert.Equal(tb, "sia", siaObj.Protocol)
		assert.Equal(tb, "testfile.dat", siaObj.ObjectKey)
		assert.Equal(tb, int64(len(data)), siaObj.Size)
		assert.NotEmpty(tb, siaObj.SiaObjectID)
	}, opt)
}

func TestRenterService_UploadObject_Staged(t *testing.T) {
	var renter *service.RenterService
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, _, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("small object")
		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "small.dat", []byte("hash123"))
		require.NoError(tb, err)

		assert.Len(tb, mockStaging.Data, 1)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "small.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, siaObj.Status)
		assert.NotEmpty(tb, siaObj.StagingKey)
		assert.Empty(tb, siaObj.SiaObjectID)
		assert.Equal(tb, int64(len(data)), siaObj.Size)
	}, opt)
}

func TestRenterService_UploadObjectMultipart_Direct(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("b"), int(slabSize*2))
		params := &core.MultipartUploadParams{
			ReaderFactory: func(start, end uint) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(data[start:end])), nil
			},
			Bucket:   "sia",
			FileName: "large.dat",
			Size:     uint64(len(data)),
		}
		err := renter.UploadObjectMultipart(context.Background(), params)
		require.NoError(tb, err)

		assert.Len(tb, mockSDK.PinnedObjs, 1)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "large.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusUploaded, siaObj.Status)
		assert.Equal(tb, int64(len(data)), siaObj.Size)
	}, opt)
}

func TestRenterService_UploadObjectMultipart_Staged(t *testing.T) {
	var renter *service.RenterService
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, _, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("tiny multipart")
		params := &core.MultipartUploadParams{
			ReaderFactory: func(start, end uint) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(data[start:end])), nil
			},
			Bucket:   "sia",
			FileName: "tiny.dat",
			Size:     uint64(len(data)),
		}
		err := renter.UploadObjectMultipart(context.Background(), params)
		require.NoError(tb, err)

		assert.Len(tb, mockStaging.Data, 1)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "tiny.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, siaObj.Status)
		assert.NotEmpty(tb, siaObj.StagingKey)
	}, opt)
}

func TestRenterService_UploadObjectMultipart_UnknownSize(t *testing.T) {
	var renter *service.RenterService
	var readerFactoryCalled bool

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("c"), int(slabSize+10))

		params := &core.MultipartUploadParams{
			ReaderFactory: func(start, end uint) (io.ReadCloser, error) {
				readerFactoryCalled = true
				if start >= uint(len(data)) {
					return io.NopCloser(bytes.NewReader(nil)), nil
				}
				actualEnd := end
				if actualEnd > uint(len(data)) || end == 0 {
					actualEnd = uint(len(data))
				}
				return io.NopCloser(bytes.NewReader(data[start:actualEnd])), nil
			},
			Bucket:   "sia",
			FileName: "unknown.dat",
			Size:     0,
		}
		err := renter.UploadObjectMultipart(context.Background(), params)
		require.NoError(tb, err)

		assert.True(tb, readerFactoryCalled)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "unknown.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusUploaded, siaObj.Status)
		assert.Equal(tb, int64(len(data)), siaObj.Size)
	}, opt)
}

func TestRenterService_GetObject_Staged(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("staged content")

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "staged.dat", []byte("hash"))
		require.NoError(tb, err)

		reader, err := renter.GetObject(context.Background(), "sia", "staged.dat", core.DownloadOptions{})
		require.NoError(tb, err)
		defer reader.Close()

		got, _ := io.ReadAll(reader)
		assert.Equal(tb, data, got)
	}, opt)
}

func TestRenterService_GetObject_Uploaded(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("d"), int(slabSize*2))

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "uploaded.dat", []byte("hash"))
		require.NoError(tb, err)

		mockSDK.DownloadData = data
		mockSDK.UnsealObj = sdk.NewEmptyObject()

		reader, err := renter.GetObject(context.Background(), "sia", "uploaded.dat", core.DownloadOptions{})
		require.NoError(tb, err)
		defer reader.Close()

		got, _ := io.ReadAll(reader)
		assert.Equal(tb, data, got)
	}, opt)
}

func TestRenterService_GetObject_Range(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("0123456789ABCDEF")

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "range.dat", []byte("hash"))
		require.NoError(tb, err)

		reader, err := renter.GetObject(context.Background(), "sia", "range.dat", core.DownloadOptions{
			Range: &core.DownloadRange{Offset: 4, Length: 4},
		})
		require.NoError(tb, err)
		defer reader.Close()

		got, _ := io.ReadAll(reader)
		assert.Equal(tb, []byte("4567"), got)
	}, opt)
}

func TestRenterService_GetObject_NotFound(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		_, err := renter.GetObject(context.Background(), "sia", "nonexistent.dat", core.DownloadOptions{})
		assert.Error(tb, err)
	}, opt)
}

func TestRenterService_DeleteObject_Staged(t *testing.T) {
	var renter *service.RenterService
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, _, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("to be deleted")

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "delete-staged.dat", []byte("hash"))
		require.NoError(tb, err)

		err = renter.DeleteObject(context.Background(), "sia", "delete-staged.dat")
		require.NoError(tb, err)

		assert.Len(tb, mockStaging.Data, 0)

		var siaObj models.RenterObject
		err = ctx.DB().Unscoped().Where("protocol = ? AND object_key = ?", "sia", "delete-staged.dat").First(&siaObj).Error
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))
	}, opt)
}

func TestRenterService_DeleteObject_Uploaded(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("e"), int(slabSize*2))

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "delete-uploaded.dat", []byte("hash"))
		require.NoError(tb, err)

		err = renter.DeleteObject(context.Background(), "sia", "delete-uploaded.dat")
		require.NoError(tb, err)

		assert.Len(tb, mockSDK.DeletedObjs, 1)

		var siaObj models.RenterObject
		err = ctx.DB().Unscoped().Where("protocol = ? AND object_key = ?", "sia", "delete-uploaded.dat").First(&siaObj).Error
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))
	}, opt)
}

func TestRenterService_DeleteObject_NotFound(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		err := renter.DeleteObject(context.Background(), "sia", "nonexistent.dat")
		require.NoError(tb, err)
	}, opt)
}

func TestRenterService_UploadExists(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		exists, siaObj, err := renter.UploadExists(context.Background(), "sia", "missing.dat")
		require.NoError(tb, err)
		assert.False(tb, exists)
		assert.Nil(tb, siaObj)

		err = renter.UploadObject(context.Background(), bytes.NewReader([]byte("data")), "sia", "exists.dat", []byte("hash"))
		require.NoError(tb, err)

		exists, siaObj, err = renter.UploadExists(context.Background(), "sia", "exists.dat")
		require.NoError(tb, err)
		assert.True(tb, exists)
		assert.NotNil(tb, siaObj)
		assert.Equal(tb, "sia", siaObj.Protocol)
		assert.Equal(tb, "exists.dat", siaObj.ObjectKey)
	}, opt)
}

func TestRenterService_GetObjectMetadata(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("metadata test")

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "meta.dat", []byte("hash"))
		require.NoError(tb, err)

		meta, err := renter.GetObjectMetadata(context.Background(), "sia", "meta.dat")
		require.NoError(tb, err)
		assert.Equal(tb, "sia", meta.Bucket)
		assert.Equal(tb, "meta.dat", meta.Key)
		assert.Equal(tb, int64(len(data)), meta.Size)

		_, err = renter.GetObjectMetadata(context.Background(), "sia", "nonexistent.dat")
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))
	}, opt)
}

func TestRenterService_OrphanCleanup_OnDBCreateFailure(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("f"), int(slabSize*2))

		// Pre-existing row in "deleting" state: objectAlreadyUploaded returns
		// false (status == deleting), so the upload proceeds to pinAndStore,
		// whose Create() hits the unique index constraint and triggers orphan
		// cleanup of the SDK object.
		conflict := models.RenterObject{
			Protocol:  "sia",
			ObjectKey: "conflict.dat",
			Bucket:    "sia",
			Status:    models.RenterObjectStatusDeleting,
		}
		require.NoError(tb, ctx.DB().Create(&conflict).Error)

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "conflict.dat", []byte("hash"))
		require.Error(tb, err)

		assert.Len(tb, mockSDK.DeletedObjs, 1)
	}, opt)
}

func TestRenterService_SlabSize(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(proto4.SectorSize)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		size, err := renter.SlabSize(context.Background())
		require.NoError(tb, err)
		assert.Equal(tb, uint64(slabSize), size)
	}, opt)
}

func TestRenterService_CreateBucketIfNotExists(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		err := renter.CreateBucketIfNotExists("any-bucket")
		require.NoError(tb, err)
	}, opt)
}

func TestRenterService_findSiaObject(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		obj1 := models.RenterObject{
			Protocol:  "sia",
			Bucket:    "sia",
			ObjectKey: "file1.dat",
			Size:      100,
			Status:    models.RenterObjectStatusUploaded,
		}
		obj2 := models.RenterObject{
			Protocol:  "sia",
			Bucket:    "sia",
			ObjectKey: "file2.dat",
			Size:      200,
			Status:    models.RenterObjectStatusStaged,
		}
		require.NoError(tb, ctx.DB().Create(&obj1).Error)
		require.NoError(tb, ctx.DB().Create(&obj2).Error)

		exists, siaObj, err := renter.UploadExists(context.Background(), "sia", "file1.dat")
		require.NoError(tb, err)
		assert.True(tb, exists)
		assert.NotNil(tb, siaObj)
		assert.Equal(tb, "file1.dat", siaObj.ObjectKey)
		assert.Equal(tb, int64(100), siaObj.Size)

		exists, siaObj, err = renter.UploadExists(context.Background(), "s3", "file1.dat")
		require.NoError(tb, err)
		assert.False(tb, exists)
		assert.Nil(tb, siaObj)

		exists, siaObj, err = renter.UploadExists(context.Background(), "sia", "nonexistent.dat")
		require.NoError(tb, err)
		assert.False(tb, exists)
		assert.Nil(tb, siaObj)
	}, opt)
}

func TestRenterService_DeleteObjectMetadata(t *testing.T) {
	var renter *service.RenterService
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, _, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("delete metadata test")

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "delmeta.dat", []byte("hash"))
		require.NoError(tb, err)

		err = renter.DeleteObjectMetadata(context.Background(), "sia", "delmeta.dat")
		require.NoError(tb, err)

		assert.Len(tb, mockStaging.Data, 0)

		var siaObj models.RenterObject
		err = ctx.DB().Unscoped().Where("protocol = ? AND object_key = ?", "sia", "delmeta.dat").First(&siaObj).Error
		assert.Error(tb, err)
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound))
	}, opt)
}

// ============================================================================
// REGRESSION TESTS — Kody PR #1629 review feedback
// ============================================================================

func TestRenterService_SDKNotConfigured_GracefulDegradation(t *testing.T) {
	renter := &service.RenterService{}
	// Do NOT call SetSDK — sdkConfigured stays false.

	err := renter.UploadObject(context.Background(), bytes.NewReader([]byte("x")), "sia", "f.dat", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDK is not configured")

	_, err = renter.GetObject(context.Background(), "sia", "f.dat", core.DownloadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDK is not configured")

	_, err = renter.GetObjectMetadata(context.Background(), "sia", "f.dat")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDK is not configured")

	err = renter.DeleteObject(context.Background(), "sia", "f.dat")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDK is not configured")

	_, _, err = renter.UploadExists(context.Background(), "sia", "f.dat")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDK is not configured")

	_, err = renter.SlabSize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDK is not configured")
}

func TestRenterService_UploadDirect_PinFailure_OrphanCleanup(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockSDK.PinErr = errors.New("pin failed")

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("a"), int(slabSize*2))
		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "pinfail.dat", []byte("hash"))
		require.Error(tb, err)
		assert.Contains(tb, err.Error(), "failed to pin object")

		assert.Len(tb, mockSDK.DeletedObjs, 1)
	}, opt)
}

func TestRenterService_UploadMultipart_PinFailure_OrphanCleanup(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockSDK.PinErr = errors.New("pin failed")

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("b"), int(slabSize*2))
		params := &core.MultipartUploadParams{
			ReaderFactory: func(start, end uint) (io.ReadCloser, error) {
				actualEnd := end
				if actualEnd > uint(len(data)) {
					actualEnd = uint(len(data))
				}
				return io.NopCloser(bytes.NewReader(data[start:actualEnd])), nil
			},
			Bucket:   "sia",
			FileName: "multipartpinfail.dat",
			Size:     uint64(len(data)),
		}
		err := renter.UploadObjectMultipart(context.Background(), params)
		require.Error(tb, err)
		assert.Contains(tb, err.Error(), "failed to pin object")

		assert.Len(tb, mockSDK.DeletedObjs, 1)
	}, opt)
}

func TestRenterService_GetObject_ZeroLengthRange(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("0123456789ABCDEF")

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "zerorange.dat", []byte("hash"))
		require.NoError(tb, err)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "zerorange.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, siaObj.Status)

		reader, err := renter.GetObject(context.Background(), "sia", "zerorange.dat", core.DownloadOptions{
			Range: &core.DownloadRange{Offset: 4, Length: 0},
		})
		require.NoError(tb, err)
		defer reader.Close()

		got, _ := io.ReadAll(reader)
		assert.Equal(tb, data[4:], got)
	}, opt)
}

func TestRenterService_DeleteObject_ParseBeforeTransition(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		obj := models.RenterObject{
			Protocol:    "sia",
			Bucket:      "sia",
			ObjectKey:   "badid.dat",
			SiaObjectID: "NOT_VALID_HEX",
			Size:        100,
			Status:      models.RenterObjectStatusUploaded,
		}
		require.NoError(tb, ctx.DB().Create(&obj).Error)

		err := renter.DeleteObject(context.Background(), "sia", "badid.dat")
		require.Error(tb, err)
		assert.Contains(tb, err.Error(), "failed to parse sia_object_id")

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "badid.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusUploaded, siaObj.Status,
			"object should remain in uploaded state, not stuck in deleting")
	}, opt)
}

func TestRenterService_DeleteObject_DBDeleteBeforeSDK(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockSDK.DeleteErr = errors.New("sdk delete failed")

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("e"), int(slabSize*2))

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "dbfirst.dat", []byte("hash"))
		require.NoError(tb, err)

		err = renter.DeleteObject(context.Background(), "sia", "dbfirst.dat")
		require.NoError(tb, err)

		var siaObj models.RenterObject
		err = ctx.DB().Unscoped().Where("protocol = ? AND object_key = ?", "sia", "dbfirst.dat").First(&siaObj).Error
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound),
			"DB row should be deleted before SDK cleanup, not stuck in deleting")
	}, opt)
}

func TestRenterService_ReuploadAfterDelete(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("first upload")

		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "reupload.dat", []byte("hash1"))
		require.NoError(tb, err)

		err = renter.DeleteObject(context.Background(), "sia", "reupload.dat")
		require.NoError(tb, err)

		data2 := []byte("second upload")
		err = renter.UploadObject(context.Background(), bytes.NewReader(data2), "sia", "reupload.dat", []byte("hash2"))
		require.NoError(tb, err, "re-upload after delete should not fail with unique constraint violation")

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "reupload.dat").First(&siaObj).Error)
		assert.Equal(tb, int64(len(data2)), siaObj.Size)
	}, opt)
}

func TestRenterService_UploadObject_SeekableReader(t *testing.T) {
	var renter *service.RenterService

	slabSize := int64(1024)
	r, _, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("a"), int(slabSize*2))
		reader := bytes.NewReader(data)

		err := renter.UploadObject(context.Background(), reader, "sia", "seekable.dat", []byte("hash"))
		require.NoError(tb, err)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "seekable.dat").First(&siaObj).Error)
		assert.Equal(tb, int64(len(data)), siaObj.Size)
		assert.Equal(tb, models.RenterObjectStatusUploaded, siaObj.Status)
	}, opt)
}

func TestRenterService_UploadMultipart_SingleAddCall(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("b"), int(slabSize*3))

		params := &core.MultipartUploadParams{
			ReaderFactory: func(start, end uint) (io.ReadCloser, error) {
				actualEnd := end
				if actualEnd > uint(len(data)) {
					actualEnd = uint(len(data))
				}
				return io.NopCloser(bytes.NewReader(data[start:actualEnd])), nil
			},
			Bucket:   "sia",
			FileName: "singleadd.dat",
			Size:     uint64(len(data)),
		}
		err := renter.UploadObjectMultipart(context.Background(), params)
		require.NoError(tb, err)

		assert.Len(tb, mockSDK.Upload.AddedData, 1,
			"should use a single Add() call, not multiple — prevents orphaned packed objects")
	}, opt)
}

func TestRenterService_AppKeyDecoding(t *testing.T) {
	// A 64-byte Ed25519 private key (seed + public key) = 128 hex chars.
	validKeyHex := "de3c8c54e99aaac4070771b261a8341c1e795b1618e33cde9973b92a4c3732b3de3c8c54e99aaac4070771b261a8341c1e795b1618e33cde9973b92a4c3732b3"

	decoded, err := hex.DecodeString(validKeyHex)
	require.NoError(t, err)
	assert.Len(t, decoded, 64, "decoded key should be 64 bytes")

	appKey := types.PrivateKey(decoded)
	assert.Len(t, appKey, 64, "PrivateKey should preserve the full 64 bytes")

	_, err = hex.DecodeString("NOT_VALID_HEX")
	assert.Error(t, err, "invalid hex should fail before reaching SDK construction")
}

// Regression: uploading the same object key twice should not re-upload to Sia.
// The duplicate key constraint previously caused retry loops that multiplied
// the uploaded data (80MB → 900MB).
func TestRenterService_UploadObject_IdempotentSkip(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("a"), int(slabSize*2))

		// First upload succeeds.
		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "dup.dat", []byte("hash"))
		require.NoError(tb, err)
		assert.Len(tb, mockSDK.PinnedObjs, 1)

		// Second upload of same key should skip — no new SDK calls.
		err = renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "dup.dat", []byte("hash"))
		require.NoError(tb, err, "duplicate upload should be idempotent, not error")
		assert.Len(tb, mockSDK.PinnedObjs, 1, "should not pin a second object")
	}, opt)
}

// Regression: multipart upload of same key should be idempotent.
func TestRenterService_UploadObjectMultipart_IdempotentSkip(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("b"), int(slabSize*2))
		params := &core.MultipartUploadParams{
			ReaderFactory: func(start, end uint) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(data[start:end])), nil
			},
			Bucket:   "sia",
			FileName: "dup-multipart.dat",
			Size:     uint64(len(data)),
		}

		// First upload succeeds.
		err := renter.UploadObjectMultipart(context.Background(), params)
		require.NoError(tb, err)
		assert.Len(tb, mockSDK.PinnedObjs, 1)

		// Second upload should skip.
		err = renter.UploadObjectMultipart(context.Background(), params)
		require.NoError(tb, err, "duplicate multipart upload should be idempotent")
		assert.Len(tb, mockSDK.PinnedObjs, 1, "should not pin a second object")
	}, opt)
}

// Regression: uploading an object whose key already exists in "staged" status
// (from a prior failed/retried upload) should skip idempotently, not hit the
// unique index (Protocol, ObjectKey) and return a duplicate key error.
// Previously objectAlreadyUploaded only checked Status == "uploaded", so a
// staged row caused Create() to fail with Error 1062.
func TestRenterService_UploadObject_StagedRowIdempotentSkip(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Simulate a prior upload that staged data but never completed packing.
		mockStaging.Data["staging/existing"] = []byte("existing data")
		existing := models.RenterObject{
			Protocol:   "ipfs",
			Bucket:     "ipfs",
			ObjectKey:  "QmTLjWA9LarYchTA99s9BsJXXaXZ3dVhujEbydKsGwKQVz",
			StagingKey: "staging/existing",
			Size:       13,
			Status:     models.RenterObjectStatusStaged,
		}
		require.NoError(tb, ctx.DB().Create(&existing).Error)

		// Retry upload of the same key — should skip, not duplicate key error.
		data := bytes.Repeat([]byte("a"), int(slabSize*2))
		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "ipfs", "QmTLjWA9LarYchTA99s9BsJXXaXZ3dVhujEbydKsGwKQVz", []byte("hash"))
		require.NoError(tb, err, "upload with existing staged row should be idempotent, not duplicate key error")

		// No SDK upload/pin should have occurred.
		assert.Empty(tb, mockSDK.PinnedObjs, "should not pin when a staged row already exists")
	}, opt)
}

// Regression: same scenario for UploadObjectMultipart — a staged row should
// cause an idempotent skip, not a duplicate key error on Create().
func TestRenterService_UploadObjectMultipart_StagedRowIdempotentSkip(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		mockStaging.Data["staging/existing-mu"] = []byte("existing data")
		existing := models.RenterObject{
			Protocol:   "ipfs",
			Bucket:     "ipfs",
			ObjectKey:  "QmTLjWA9LarYchTA99s9BsJXXaXZ3dVhujEbydKsGwKQVz",
			StagingKey: "staging/existing-mu",
			Size:       13,
			Status:     models.RenterObjectStatusStaged,
		}
		require.NoError(tb, ctx.DB().Create(&existing).Error)

		data := bytes.Repeat([]byte("b"), int(slabSize*2))
		params := &core.MultipartUploadParams{
			ReaderFactory: func(start, end uint) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(data[start:end])), nil
			},
			Bucket:   "ipfs",
			FileName: "QmTLjWA9LarYchTA99s9BsJXXaXZ3dVhujEbydKsGwKQVz",
			Size:     uint64(len(data)),
		}

		err := renter.UploadObjectMultipart(context.Background(), params)
		require.NoError(tb, err, "multipart upload with existing staged row should be idempotent")
		assert.Empty(tb, mockSDK.PinnedObjs, "should not pin when a staged row already exists")
	}, opt)
}

// Verify that redundancy options are forwarded to the SDK's UploadPacked call.
func TestRenterService_RedundancyOptionsPassed(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK

	slabSize := int64(1024)
	r, sdkMock, _, opt := withRenterServiceAndMocks(slabSize)
	r.SetUploadOpts([]sdk.UploadOption{
		sdk.WithRedundancy(10, 20),
	})
	renter = r
	mockSDK = sdkMock

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := bytes.Repeat([]byte("a"), int(slabSize*2))
		err := renter.UploadObject(context.Background(), bytes.NewReader(data), "sia", "redundancy.dat", []byte("hash"))
		require.NoError(tb, err)

		assert.NotEmpty(tb, mockSDK.UploadOpts, "upload options should be passed to UploadPacked")
		assert.Len(tb, mockSDK.UploadOpts, 1, "should pass exactly one upload option")
	}, opt)
}
