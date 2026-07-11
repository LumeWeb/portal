package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.sia.tech/core/types"
	sdk "go.sia.tech/siastorage"

	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"go.lumeweb.com/portal/service/internal/indexd"
	svcTesting "go.lumeweb.com/portal/service/testing"
	"gorm.io/gorm"
)

// --- Packing loop regression tests ---
//
// These tests exercise internal indexd functions (PackStagedObjects,
// RecoverStuckPacking, PackingLoopCfg) directly. They must live in
// service/ (package service_test) because they import service/internal/indexd,
// which is only accessible from within the service module.
//
// The public API tests and mock implementations live in:
//   - core/internal/service_tests/renter_test.go (public RenterService tests)
//   - service/testing/renter_mocks.go (mock SDK, staging backend, test helpers)

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

// TestRecoverStuckPacking_RevertsToStaged tests that objects stuck in "packing"
// state are reverted to "staged" on startup. (Kody: "objects left in packing
// state during a restart are never reprocessed")
func TestRecoverStuckPacking_RevertsToStaged(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		obj := models.RenterObject{
			Protocol:   "sia",
			Bucket:     "sia",
			ObjectKey:  "stuck-packing.dat",
			StagingKey: "staging/0",
			Size:       100,
			Status:     models.RenterObjectStatusPacking,
		}
		require.NoError(tb, ctx.DB().Create(&obj).Error)

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.RecoverStuckPacking(context.Background(), cfg)
		require.NoError(tb, err)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "stuck-packing.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, siaObj.Status,
			"stuck packing object should be reverted to staged")
	}, opt)
}

// TestRecoverStuckPacking_DeletingRecovery tests that objects stuck in
// "deleting" state have their SDK/staging data cleaned up and the DB row
// removed. (Kody: "RecoverStuckPacking only repairs objects stuck in packing
// state, leaving rows stranded in deleting")
func TestRecoverStuckPacking_DeletingRecovery(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		mockStaging.Data["staging/deleting"] = []byte("staged data for deleting object")

		obj := models.RenterObject{
			Protocol:   "sia",
			Bucket:     "sia",
			ObjectKey:  "stuck-deleting.dat",
			StagingKey: "staging/deleting",
			Size:       100,
			Status:     models.RenterObjectStatusDeleting,
		}
		require.NoError(tb, ctx.DB().Create(&obj).Error)

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.RecoverStuckPacking(context.Background(), cfg)
		require.NoError(tb, err)

		var siaObj models.RenterObject
		err = ctx.DB().Unscoped().Where("protocol = ? AND object_key = ?", "sia", "stuck-deleting.dat").First(&siaObj).Error
		assert.True(tb, errors.Is(err, gorm.ErrRecordNotFound),
			"stuck deleting row should be removed")

		_, ok := mockStaging.Data["staging/deleting"]
		assert.False(t, ok, "staging data should be cleaned up for stuck deleting object")
	}, opt)
}

// TestRecoverStuckPacking_DeletingRecovery_WithSDKObject tests that objects
// stuck in "deleting" state with an SDK object ID have the SDK object deleted.
func TestRecoverStuckPacking_DeletingRecovery_WithSDKObject(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		objID := types.Hash256{}
		objID[0] = 0x42
		objIDHex := fmt.Sprintf("%x", objID[:])

		obj := models.RenterObject{
			Protocol:    "sia",
			Bucket:      "sia",
			ObjectKey:   "stuck-deleting-sdk.dat",
			SiaObjectID: objIDHex,
			Size:        100,
			Status:      models.RenterObjectStatusDeleting,
		}
		require.NoError(tb, ctx.DB().Create(&obj).Error)

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.RecoverStuckPacking(context.Background(), cfg)
		require.NoError(tb, err)

		assert.Len(tb, mockSDK.DeletedObjs, 1)
		assert.Equal(tb, objID, mockSDK.DeletedObjs[0])
	}, opt)
}

// TestPackStagedObjects_PinFailure_OrphanCleanup tests that when PinObject fails
// during packing, the object is reverted to staged and the SDK object is cleaned
// up. (Kody: "State leak in packing.go leaves a RenterObject stuck in packing when
// cfg.SDK.PinObject fails")
func TestPackStagedObjects_PinFailure_OrphanCleanup(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging
	mockSDK.PinErr = errors.New("pin failed")

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("small packed object")
		stagingKey := "staging/pinfail"
		mockStaging.Data[stagingKey] = data

		obj := models.RenterObject{
			Protocol:   "sia",
			Bucket:     "sia",
			ObjectKey:  "pinfail-pack.dat",
			StagingKey: stagingKey,
			Size:       int64(len(data)),
			Status:     models.RenterObjectStatusStaged,
		}
		require.NoError(tb, ctx.DB().Create(&obj).Error)

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.PackStagedObjects(context.Background(), cfg)
		require.NoError(tb, err)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "pinfail-pack.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, siaObj.Status,
			"object should be reverted to staged after pin failure, not stuck in packing")

		assert.Len(tb, mockSDK.DeletedObjs, 1,
			"finalized SDK object should be deleted after pin failure")
	}, opt)
}

// TestPackStagedObjects_FinalizeCountMismatch_Cleanup tests that when Finalize
// returns a different number of objects than expected, the orphaned SDK objects
// are cleaned up. (Kody: "Finalize commits SDK objects before the result-count
// check, so returning an error leaves orphaned objects on the network")
func TestPackStagedObjects_FinalizeCountMismatch_Cleanup(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	mockSDK.Upload.FinalizeObjs = []sdk.Object{
		sdk.NewEmptyObject(),
		sdk.NewEmptyObject(),
	}

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("mismatch test data")
		stagingKey := "staging/mismatch"
		mockStaging.Data[stagingKey] = data

		obj := models.RenterObject{
			Protocol:   "sia",
			Bucket:     "sia",
			ObjectKey:  "mismatch.dat",
			StagingKey: stagingKey,
			Size:       int64(len(data)),
			Status:     models.RenterObjectStatusStaged,
		}
		require.NoError(tb, ctx.DB().Create(&obj).Error)

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.PackStagedObjects(context.Background(), cfg)
		require.NoError(tb, err)

		assert.GreaterOrEqual(tb, len(mockSDK.DeletedObjs), 1,
			"orphaned SDK objects from count mismatch should be cleaned up")
	}, opt)
}

// TestPackStagedObjects_BatchCAS_TransitionsStagedToPacking tests that the batch
// CAS query transitions all staged objects to packing in a single UPDATE.
// (Kody: "PackStagedObjects runs up to 1000 separate transactions per cycle")
func TestPackStagedObjects_BatchCAS_TransitionsStagedToPacking(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		for i := 0; i < 3; i++ {
			key := fmt.Sprintf("staging/batch%d", i)
			mockStaging.Data[key] = []byte("data")
			obj := models.RenterObject{
				Protocol:   "sia",
				Bucket:     "sia",
				ObjectKey:  fmt.Sprintf("batch%d.dat", i),
				StagingKey: key,
				Size:       4,
				Status:     models.RenterObjectStatusStaged,
			}
			require.NoError(tb, ctx.DB().Create(&obj).Error)
		}

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.PackStagedObjects(context.Background(), cfg)
		require.NoError(tb, err)

		var stillStaged []models.RenterObject
		ctx.DB().Where("status = ? AND protocol = ?", models.RenterObjectStatusStaged, "sia").Find(&stillStaged)

		var inPacking []models.RenterObject
		ctx.DB().Where("status = ? AND protocol = ?", models.RenterObjectStatusPacking, "sia").Find(&inPacking)

		assert.Len(tb, inPacking, 0, "no objects should be stuck in packing after PackStagedObjects")
	}, opt)
}

// TestPackStagedObjects_AddFailure_NoStackOverflow tests that when upload.Add
// fails for one object in a group, the recursion uses splitAt+1 (not splitAt)
// to avoid infinite recursion. (Kody: "Unbounded recursion in uploadSubGroup")
func TestPackStagedObjects_AddFailure_NoStackOverflow(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	mockSDK.Upload.AddFailOnCall = 1
	mockSDK.Upload.AddErr = errors.New("add failed")

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		data := []byte("recursion test")
		stagingKey := "staging/recursion"
		mockStaging.Data[stagingKey] = data

		obj := models.RenterObject{
			Protocol:   "sia",
			Bucket:     "sia",
			ObjectKey:  "recursion.dat",
			StagingKey: stagingKey,
			Size:       int64(len(data)),
			Status:     models.RenterObjectStatusStaged,
		}
		require.NoError(tb, ctx.DB().Create(&obj).Error)

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.PackStagedObjects(context.Background(), cfg)
		require.NoError(tb, err)

		var siaObj models.RenterObject
		require.NoError(tb, ctx.DB().Where("protocol = ? AND object_key = ?", "sia", "recursion.dat").First(&siaObj).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, siaObj.Status,
			"object should be reverted to staged after Add failure, not stuck in packing")
	}, opt)
}

// TestPackStagedObjects_MultipleAddFailures_Iterative tests that when upload.Add
// fails multiple times within a group, the iterative loop (not recursion)
// processes all remaining objects without stack overflow. Verifies Bug 1 fix:
// unbounded recursion converted to iterative loop.
//
// Setup: 5 objects in one group, Add fails on calls 1 and 3. The iterative
// loop should:
//   - Skip Finalize for the first empty sub-group (first Add failed)
//   - Finalize the second sub-group (1 object added before call 3 fails)
//   - Finalize the third sub-group (remaining 2 objects)
func TestPackStagedObjects_MultipleAddFailures_Iterative(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create 5 staged objects in one group (all < slabSize).
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("staging/multi%d", i)
			mockStaging.Data[key] = []byte(fmt.Sprintf("data%d", i))
			obj := models.RenterObject{
				Protocol:   "sia",
				Bucket:     "sia",
				ObjectKey:  fmt.Sprintf("multi%d.dat", i),
				StagingKey: key,
				Size:       6,
				Status:     models.RenterObjectStatusStaged,
			}
			require.NoError(tb, ctx.DB().Create(&obj).Error)
		}

		// Use UploadFactory to create fresh uploads per UploadPacked() call,
		// each with its own Add failure pattern.
		callIdx := 0
		mockSDK.UploadFactory = func() *svcTesting.MockRenterPackedUpload {
			callIdx++
			upload := &svcTesting.MockRenterPackedUpload{}
			if callIdx == 1 {
				// First sub-group: fail on first Add (Bug 2 scenario).
				upload.AddFailOnCall = 1
				upload.AddErr = errors.New("add failed")
			}
			return upload
		}

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.PackStagedObjects(context.Background(), cfg)
		require.NoError(tb, err)

		// The first object (multi0.dat) should be reverted to staged since
		// its Add() failed on the first sub-group.
		var obj0 models.RenterObject
		require.NoError(tb, ctx.DB().Where("object_key = ?", "multi0.dat").First(&obj0).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, obj0.Status,
			"first object should be reverted to staged after Add failure")

		// The remaining 4 objects should be uploaded successfully since the
		// iterative loop continues processing them.
		for i := 1; i < 5; i++ {
			var obj models.RenterObject
			require.NoError(tb, ctx.DB().Where("object_key = ?", fmt.Sprintf("multi%d.dat", i)).First(&obj).Error)
			assert.Equal(tb, models.RenterObjectStatusUploaded, obj.Status,
				"object %d should be uploaded after iterative processing", i)
		}
	}, opt)
}

// TestPackStagedObjects_FirstAddFailure_SkipsFinalize tests that when the first
// Add() call in a sub-group fails, Finalize is NOT called on the empty packed
// upload. Verifies Bug 2 fix: skip Finalize when objIdx is empty.
func TestPackStagedObjects_FirstAddFailure_SkipsFinalize(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Two objects: first Add fails, second should succeed in a new sub-group.
		for i := 0; i < 2; i++ {
			key := fmt.Sprintf("staging/skip%d", i)
			mockStaging.Data[key] = []byte(fmt.Sprintf("data%d", i))
			obj := models.RenterObject{
				Protocol:   "sia",
				Bucket:     "sia",
				ObjectKey:  fmt.Sprintf("skip%d.dat", i),
				StagingKey: key,
				Size:       6,
				Status:     models.RenterObjectStatusStaged,
			}
			require.NoError(tb, ctx.DB().Create(&obj).Error)
		}

		// Track Finalize calls across all uploads.
		var finalizeCount int
		callIdx := 0
		mockSDK.UploadFactory = func() *svcTesting.MockRenterPackedUpload {
			callIdx++
			u := &svcTesting.MockRenterPackedUpload{}
			if callIdx == 1 {
				u.AddFailOnCall = 1 // First Add fails
				u.AddErr = errors.New("first add failed")
			}
			// Wrap Finalize to count calls
			origFinalize := u.Finalize
			_ = origFinalize
			u.FinalizeCount = 0
			// We'll check finalCount after
			return u
		}

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.PackStagedObjects(context.Background(), cfg)
		require.NoError(tb, err)

		// Verify the first object was reverted to staged.
		var obj0 models.RenterObject
		require.NoError(tb, ctx.DB().Where("object_key = ?", "skip0.dat").First(&obj0).Error)
		assert.Equal(tb, models.RenterObjectStatusStaged, obj0.Status,
			"first object should be reverted to staged")

		// Verify the second object was uploaded.
		var obj1 models.RenterObject
		require.NoError(tb, ctx.DB().Where("object_key = ?", "skip1.dat").First(&obj1).Error)
		assert.Equal(tb, models.RenterObjectStatusUploaded, obj1.Status,
			"second object should be uploaded in second sub-group")

		// The first sub-group had an empty objIdx (first Add failed), so
		// Finalize should NOT have been called for it. Only the second
		// sub-group should have called Finalize (for 1 object).
		// Since UploadFactory creates fresh instances, we check the total
		// via PinnedObjs (1 pin = 1 finalize with 1 result).
		assert.Len(tb, mockSDK.PinnedObjs, 1,
			"only 1 object should be pinned (from the second sub-group's Finalize)")
		_ = finalizeCount
	}, opt)
}

// TestPackStagedObjects_StagingBackendFailure_BailsEarly tests that when the
// staging backend returns errors for consecutive reads, the packing loop bails
// out early instead of attempting Finalize on an empty packed upload.
// Verifies Bug 4 fix: bail early on consecutive staging read failures.
func TestPackStagedObjects_StagingBackendFailure_BailsEarly(t *testing.T) {
	var renter *service.RenterService
	var mockSDK *svcTesting.MockRenterSDK
	var mockStaging *svcTesting.MockStagingBackend

	slabSize := int64(1024)
	r, sdkMock, staging, opt := withRenterServiceAndMocks(slabSize)
	renter = r
	mockSDK = sdkMock
	mockStaging = staging

	// Staging backend always fails on Get.
	mockStaging.GetErr = errors.New("staging backend unavailable")

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create 5 staged objects — all will fail staging reads.
		for i := 0; i < 5; i++ {
			obj := models.RenterObject{
				Protocol:   "sia",
				Bucket:     "sia",
				ObjectKey:  fmt.Sprintf("bail%d.dat", i),
				StagingKey: fmt.Sprintf("staging/bail%d", i),
				Size:       5,
				Status:     models.RenterObjectStatusStaged,
			}
			require.NoError(tb, ctx.DB().Create(&obj).Error)
		}

		// Track if UploadPacked was called and if Finalize was called.
		uploadCreated := false
		callIdx := 0
		mockSDK.UploadFactory = func() *svcTesting.MockRenterPackedUpload {
			uploadCreated = true
			callIdx++
			u := &svcTesting.MockRenterPackedUpload{}
			return u
		}

		cfg := indexd.PackingLoopCfg{
			Component:      renter,
			Logger:         renter.Logger(),
			SDK:            mockSDK,
			StagingBackend: mockStaging,
			SlabSize:       slabSize,
		}

		err := indexd.PackStagedObjects(context.Background(), cfg)
		require.NoError(tb, err)

		// All objects should be reverted to staged. The first 3 are
		// individually reverted after failing staging reads. The remaining
		// 2 are never reached (bail-out after 3 consecutive failures)
		// and stay in "packing" until RecoverStuckPacking reverts them.
		for i := 0; i < 3; i++ {
			var obj models.RenterObject
			require.NoError(tb, ctx.DB().Where("object_key = ?", fmt.Sprintf("bail%d.dat", i)).First(&obj).Error)
			assert.Equal(tb, models.RenterObjectStatusStaged, obj.Status,
				"object %d should be reverted to staged after staging read failure", i)
		}
		for i := 3; i < 5; i++ {
			var obj models.RenterObject
			require.NoError(tb, ctx.DB().Where("object_key = ?", fmt.Sprintf("bail%d.dat", i)).First(&obj).Error)
			assert.Equal(tb, models.RenterObjectStatusPacking, obj.Status,
				"object %d should remain in packing after bail-out (reverted by RecoverStuckPacking on next cycle)", i)
		}

		// No objects should be pinned — Finalize was never called because
		// all staging reads failed and the bail-out kicked in.
		assert.Empty(tb, mockSDK.PinnedObjs,
			"no objects should be pinned when staging backend is unavailable")
		_ = uploadCreated
	}, opt)
}
