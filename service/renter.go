package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service/internal/indexd"
	"go.opentelemetry.io/otel/attribute"
	proto4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	sdk "go.sia.tech/siastorage"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.RENTER_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewRenterService()
		},
		Metrics: indexd.GetCollectors(),
	})
}

// Compile-time assertion that RenterService implements core.RenterService.
var _ core.RenterService = (*RenterService)(nil)

// RenterService implements core.RenterService using the siastorage SDK
// and indexd.
//
// Small objects (< slabSize) are staged via a StagingBackend and packed into
// slabs by a background loop. Large objects (>= slabSize) are uploaded
// directly via UploadPacked.
//
// The implementation (FSM, packing loop, staging backend) lives in
// service/internal/indexd. This struct is the thin public wrapper that
// satisfies core.RenterService and delegates to the internal package.
type RenterService struct {
	*core.BaseComponent

	sdk            indexd.SDK
	sdkConfigured  bool
	stagingBackend indexd.StagingBackend
	slabSize       int64
	uploadOpts     []sdk.UploadOption

	packingLoopCtx    context.Context
	packingLoopCancel context.CancelFunc
	packingLoopDone   chan struct{}

	syncLoopCtx    context.Context
	syncLoopCancel context.CancelFunc
	syncLoopDone   chan struct{}
}

// NewRenterService creates a new RenterService. The SDK, staging
// backend, and slab size must be provided by the caller during initialization.
func NewRenterService() (*RenterService, []core.ContextBuilderOption, error) {
	renter := &RenterService{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			if err := renter.Init(); err != nil {
				return fmt.Errorf("failed to initialize indexd renter service: %w", err)
			}
			return nil
		}),
	)

	return renter, opts, nil
}

func (r *RenterService) ID() string {
	return core.RENTER_SERVICE
}

func (r *RenterService) Init() error {
	ctx := r.Context()
	if r.slabSize <= 0 {
		r.slabSize = int64(proto4.SectorSize)
	}

	// Auto-construct the SDK from config if not explicitly provided (tests
	// call SetSDK before init to inject a mock).
	if r.sdk == nil {
		siaCfg := r.Config().Config().Core.Storage.Sia
		if siaCfg.AppKey == "" || siaCfg.URL == "" {
			r.Logger().Error("indexd SDK is not configured -- run 'portal sia login' to set it up. " +
				"All renter operations will return errors until configured.")
			return nil
		}
		appKeyBytes, err := hex.DecodeString(siaCfg.AppKey)
		if err != nil {
			return fmt.Errorf("invalid indexd app key: %w", err)
		}
		appKey := types.PrivateKey(appKeyBytes)

		builder := sdk.NewBuilder(siaCfg.URL, core.PortalAppMetadata())
		sdkClient, err := builder.SDK(appKey, sdk.WithLogger(r.Logger().Logger))
		if err != nil {
			return fmt.Errorf("failed to create indexd SDK: %w", err)
		}
		r.sdk = &indexd.SDKAdapter{Inner: sdkClient, AppKey: appKey}
	}
	r.sdkConfigured = true

	// Build upload options from config.
	siaCfg := r.Config().Config().Core.Storage.Sia
	if siaCfg.DataShards > 0 && siaCfg.ParityShards > 0 {
		r.uploadOpts = []sdk.UploadOption{
			sdk.WithRedundancy(siaCfg.DataShards, siaCfg.ParityShards),
		}
		r.Logger().Info("configured Sia redundancy",
			zap.Uint8("dataShards", siaCfg.DataShards),
			zap.Uint8("parityShards", siaCfg.ParityShards),
		)
	}

	// Auto-create the staging backend if not explicitly set.
	if r.stagingBackend == nil {
		siaCfg := r.Config().Config().Core.Storage.Sia
		switch siaCfg.StagingType {
		case "memory":
			r.stagingBackend = indexd.NewMemoryStagingBackend()
			r.Logger().Info("using in-memory staging backend (not durable across restarts)")
		default: // "s3" or "" (empty defaults to s3)
			s3Cfg := r.Config().Config().Core.Storage.S3
			if s3Cfg.Endpoint == "" || s3Cfg.BufferBucket == "" {
				return fmt.Errorf("staging backend is not configured and no S3 fallback config found")
			}
			sb, err := indexd.NewS3StagingBackend(ctx.GetContext(), siaCfg, s3Cfg, ensureHttpPrefix)
			if err != nil {
				return fmt.Errorf("failed to create staging backend: %w", err)
			}
			r.stagingBackend = sb
		}
	}

	// Start the background packing loop (idempotent — skip if already running).
	if r.packingLoopCancel == nil {
		r.packingLoopCtx, r.packingLoopCancel = context.WithCancel(ctx.GetContext())
		r.packingLoopDone = make(chan struct{})
		packingCfg := indexd.PackingLoopCfg{
			Component:      r,
			Logger:         r.Logger(),
			SDK:            r.sdk,
			StagingBackend: r.stagingBackend,
			SlabSize:       r.slabSize,
		}
		go indexd.PackingLoop(r.packingLoopCtx, packingCfg, r.packingLoopDone)
	}

	// Start the sealed-data refresh loop. This periodically syncs object
	// events from the indexer to keep sealed_data fresh (slab locations
	// can change as hosts go up/down). Only run if the SDK is configured.
	// Idempotent — skip if already running.
	if r.sdkConfigured && r.syncLoopCancel == nil {
		r.syncLoopCtx, r.syncLoopCancel = context.WithCancel(ctx.GetContext())
		r.syncLoopDone = make(chan struct{})
		syncCfg := indexd.SyncLoopCfg{
			Component: r,
			Logger:    r.Logger(),
			SDK:       r.sdk,
		}
		go indexd.SealedDataSyncLoop(r.syncLoopCtx, syncCfg, r.syncLoopDone)
	}

	return nil
}

// errSDKNotConfigured is returned when the SDK is not configured.
var errSDKNotConfigured = errors.New("indexd SDK is not configured -- run 'portal sia login'")

// SetSDK configures the siastorage SDK client. Must be called before Init.
func (r *RenterService) SetSDK(sdk indexd.SDK) {
	r.sdk = sdk
	r.sdkConfigured = true
}

// SetStagingBackend configures the staging backend for small objects. Must be
// called before Init.
func (r *RenterService) SetStagingBackend(sb indexd.StagingBackend) {
	r.stagingBackend = sb
}

// SetSlabSize configures the slab size threshold. Must be called before Init.
func (r *RenterService) SetSlabSize(size int64) {
	r.slabSize = size
}

// SetUploadOpts configures upload options (e.g. redundancy). Must be called
// before Init.
func (r *RenterService) SetUploadOpts(opts []sdk.UploadOption) {
	r.uploadOpts = opts
}

// CreateBucketIfNotExists is a no-op. indexd has no bucket concept.
func (r *RenterService) CreateBucketIfNotExists(bucket string) error {
	return nil
}

// ensureSDK returns errSDKNotConfigured if the SDK was not set up during init.
func (r *RenterService) ensureSDK() error {
	if !r.sdkConfigured {
		return errSDKNotConfigured
	}
	return nil
}

// UploadObject uploads an object to Sia via the siastorage SDK. Objects larger
// than or equal to the slab size are uploaded directly. Smaller objects are
// staged and packed by the background loop.
func (r *RenterService) UploadObject(ctx context.Context, file io.Reader, bucket string, fileName string, hash []byte) error {
	ctx, span := core.TraceMethod(ctx, "RenterService.UploadObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
		attribute.Int64("renter.slabSize", r.slabSize),
	)
	if err := r.ensureSDK(); err != nil {
		return err
	}
	// If the reader is seekable, stat it for size without buffering.
	if rs, ok := file.(io.ReadSeeker); ok {
		if pos, err := rs.Seek(0, io.SeekEnd); err == nil {
			if _, err := rs.Seek(0, io.SeekStart); err == nil {
				if pos < r.slabSize {
					return r.uploadStaged(ctx, rs, bucket, fileName, pos, hash)
				}
				return r.uploadDirect(ctx, rs, bucket, fileName, hash)
			}
		}
	}

	// Non-seekable reader: buffer to a temp file to determine size.
	tmpFile, err := os.CreateTemp("", "indexd-upload-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	n, err := io.Copy(tmpFile, file)
	if err != nil {
		return fmt.Errorf("failed to buffer upload: %w", err)
	}

	// Route based on size: small objects are staged for packing, large ones
	// are uploaded directly.
	if n < r.slabSize {
		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek temp file: %w", err)
		}
		return r.uploadStaged(ctx, tmpFile, bucket, fileName, n, hash)
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek temp file: %w", err)
	}
	return r.uploadDirect(ctx, tmpFile, bucket, fileName, hash)
}

// findRenterObject looks up a RenterObject by protocol and object key.
func (r *RenterService) findRenterObject(ctx context.Context, bucket, fileName string) (*models.RenterObject, error) {
	ctx, span := core.TraceMethod(ctx, "RenterService.findRenterObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
	)
	var obj models.RenterObject
	if err := r.DB().WithContext(ctx).Where("protocol = ? AND object_key = ?", bucket, fileName).First(&obj).Error; err != nil {
		return nil, err
	}
	return &obj, nil
}

// objectAlreadyUploaded returns true if a RenterObject exists in any state
// other than "deleting". This makes uploads idempotent — concurrent or retried
// uploads of the same key short-circuit instead of re-uploading and hitting a
// duplicate key constraint on the unique index (Protocol, ObjectKey).
//
// A row in "staged" or "packing" means the object is already in-flight: the
// data was written to the staging backend (staged) or is being uploaded
// (packing). Re-uploading would duplicate the work and violate the unique
// constraint. A row in "deleting" means the object is being torn down — the
// new upload should be allowed to proceed and create a fresh row.
func (r *RenterService) objectAlreadyUploaded(ctx context.Context, bucket, fileName string) bool {
	existing, err := r.findRenterObject(ctx, bucket, fileName)
	if err != nil {
		return false
	}
	return existing.Status != models.RenterObjectStatusDeleting
}

// pinAndStore pins a finalized SDK object, seals it, and persists a
// RenterObject row. On any failure after pinning, the SDK object is deleted
// to prevent orphans. Called by both uploadDirect and UploadObjectMultipart.
func (r *RenterService) pinAndStore(ctx context.Context, obj sdk.Object, bucket, fileName string, hash []byte, size int64) error {
	ctx, span := core.TraceMethod(ctx, "RenterService.pinAndStore")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
		attribute.Int64("renter.size", size),
		attribute.String("renter.siaObjectID", obj.ID().String()),
	)

	// Embed protocol/key metadata before pinning so the indexer can attribute
	// the object back to the portal. PinObject calls Seal(), which encrypts
	// the metadata with the app key.
	if err := indexd.SetObjectMetadata(&obj, bucket, fileName); err != nil {
		return fmt.Errorf("failed to set object metadata: %w", err)
	}

	if err := r.sdk.PinObject(ctx, obj); err != nil {
		if delErr := r.sdk.DeleteObject(ctx, obj.ID()); delErr != nil {
			r.Logger().Error("failed to cleanup finalized object after pin failure",
				zap.String("siaObjectID", obj.ID().String()), zap.Error(delErr))
		}
		return fmt.Errorf("failed to pin object: %w", err)
	}

	sealed := r.sdk.SealObject(obj)
	sealedJSON, err := json.Marshal(sealed)
	if err != nil {
		if delErr := r.sdk.DeleteObject(ctx, obj.ID()); delErr != nil {
			r.Logger().Error("failed to cleanup pinned object after marshal failure",
				zap.String("siaObjectID", obj.ID().String()), zap.Error(delErr))
		}
		return fmt.Errorf("failed to marshal sealed object: %w", err)
	}

	siaObjectID := obj.ID().String()
	siaObj := models.RenterObject{
		Protocol:    bucket,
		Bucket:      bucket,
		ObjectKey:   fileName,
		SiaObjectID: siaObjectID,
		SealedData:  datatypes.JSON(sealedJSON),
		Size:        size,
		Status:      models.RenterObjectStatusUploaded,
	}

	if err := db.RetryableComponentLock(r, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Create(&siaObj)
	}); err != nil {
		if delErr := r.sdk.DeleteObject(ctx, obj.ID()); delErr != nil {
			r.Logger().Error("failed to cleanup pinned object after DB error",
				zap.String("siaObjectID", siaObjectID), zap.Error(delErr))
		}
		return fmt.Errorf("failed to create sia_object record: %w", err)
	}

	r.Logger().Info("uploaded object to Sia",
		zap.String("bucket", bucket),
		zap.String("objectKey", fileName),
		zap.String("siaObjectID", siaObjectID),
		zap.Int64("size", size),
	)
	return nil
}

// uploadDirect uploads a single object via UploadPacked, pins it, and stores
// the sealed object in the database.
func (r *RenterService) uploadDirect(ctx context.Context, reader io.Reader, bucket string, fileName string, hash []byte) error {
	ctx, span := core.TraceMethod(ctx, "RenterService.uploadDirect")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
	)
	if r.objectAlreadyUploaded(ctx, bucket, fileName) {
		r.Logger().Debug("object already uploaded, skipping",
			zap.String("bucket", bucket),
			zap.String("objectKey", fileName),
		)
		return nil
	}

	upload, err := r.sdk.UploadPacked(ctx, r.uploadOpts...)
	if err != nil {
		return fmt.Errorf("failed to create packed upload: %w", err)
	}
	defer upload.Close()

	n, err := upload.Add(ctx, reader)
	if err != nil {
		return fmt.Errorf("failed to add object to packed upload: %w", err)
	}

	objects, err := upload.Finalize(ctx)
	if err != nil {
		return fmt.Errorf("failed to finalize packed upload: %w", err)
	}
	if len(objects) == 0 {
		return fmt.Errorf("packed upload finalized with no objects")
	}

	return r.pinAndStore(ctx, objects[0], bucket, fileName, hash, n)
}

// uploadStaged stores a small object in the staging backend and records a
// RenterObject row with an empty SiaObjectID. The background packing loop will
// pick it up, pack it into a slab, and update the row.
func (r *RenterService) uploadStaged(ctx context.Context, reader io.Reader, bucket string, fileName string, size int64, hash []byte) error {
	ctx, span := core.TraceMethod(ctx, "RenterService.uploadStaged")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
		attribute.Int64("renter.size", size),
	)
	if r.objectAlreadyUploaded(ctx, bucket, fileName) {
		r.Logger().Debug("object already uploaded, skipping staging",
			zap.String("bucket", bucket),
			zap.String("objectKey", fileName),
		)
		return nil
	}

	stagingKey, err := r.stagingBackend.Put(ctx, reader)
	if err != nil {
		return fmt.Errorf("failed to stage object: %w", err)
	}

	siaObj := models.RenterObject{
		Protocol:   bucket,
		Bucket:     bucket,
		ObjectKey:  fileName,
		StagingKey: stagingKey,
		Size:       size,
		Status:     models.RenterObjectStatusStaged,
	}

	if err := db.RetryableComponentLock(r, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Create(&siaObj)
	}); err != nil {
		// Best effort cleanup of staged data if DB write fails.
		if delErr := r.stagingBackend.Delete(ctx, stagingKey); delErr != nil {
			r.Logger().Error("failed to cleanup staged object after DB error",
				zap.String("stagingKey", stagingKey),
				zap.Error(delErr),
			)
		}
		return fmt.Errorf("failed to create sia_object record: %w", err)
	}

	r.Logger().Debug("staged small object for packing",
		zap.String("bucket", bucket),
		zap.String("objectKey", fileName),
		zap.String("stagingKey", stagingKey),
		zap.Int64("size", size),
	)

	return nil
}

// UploadObjectMultipart uploads a large object by streaming from a
// ReaderFactory. The data is streamed directly into UploadPacked().Add().
func (r *RenterService) UploadObjectMultipart(ctx context.Context, params *core.MultipartUploadParams) error {
	ctx, span := core.TraceMethod(ctx, "RenterService.UploadObjectMultipart")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", params.Bucket),
		attribute.String("renter.objectKey", params.FileName),
		attribute.Int64("renter.size", int64(params.Size)),
	)
	if err := r.ensureSDK(); err != nil {
		return err
	}
	bucket := params.Bucket
	fileName := params.FileName
	size := int64(params.Size)

	if r.objectAlreadyUploaded(ctx, bucket, fileName) {
		r.Logger().Debug("object already uploaded, skipping multipart upload",
			zap.String("bucket", bucket),
			zap.String("objectKey", fileName),
		)
		return nil
	}

	// If the total size is below the slab threshold, stage it.
	if size > 0 && size < r.slabSize {
		reader, err := params.ReaderFactory(0, uint(size))
		if err != nil {
			return fmt.Errorf("failed to create reader: %w", err)
		}
		defer reader.Close()
		return r.uploadStaged(ctx, reader, bucket, fileName, size, params.Hash)
	}

	// Stream the entire file through a single Add() call -- UploadPacked
	// handles internal slab chunking. Multiple Add() calls produce multiple
	// objects from Finalize, but only objects[0] is persisted, orphaning the
	// rest on Sia.
	// For unknown-size (size<=0) streams, pass a large end so the reader
	// returns all available data; upload.Add reads until EOF regardless.
	end := uint(size)
	if size <= 0 {
		end = ^uint(0) // max uint -- reader returns all available data
	}
	reader, err := params.ReaderFactory(0, end)
	if err != nil {
		return fmt.Errorf("failed to create reader: %w", err)
	}
	defer reader.Close()

	upload, err := r.sdk.UploadPacked(ctx, r.uploadOpts...)
	if err != nil {
		return fmt.Errorf("failed to create packed upload: %w", err)
	}
	defer upload.Close()

	totalWritten, err := upload.Add(ctx, reader)
	if err != nil {
		return fmt.Errorf("failed to add object to packed upload: %w", err)
	}

	objects, err := upload.Finalize(ctx)
	if err != nil {
		return fmt.Errorf("failed to finalize packed upload: %w", err)
	}
	if len(objects) == 0 {
		return fmt.Errorf("packed upload finalized with no objects")
	}

	return r.pinAndStore(ctx, objects[0], bucket, fileName, params.Hash, totalWritten)
}

// GetObject retrieves an object from Sia or the staging backend, depending
// on its state.
func (r *RenterService) GetObject(ctx context.Context, bucket string, fileName string, options core.DownloadOptions) (io.ReadCloser, error) {
	ctx, span := core.TraceMethod(ctx, "RenterService.GetObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
	)
	if err := r.ensureSDK(); err != nil {
		return nil, err
	}
	siaObj, err := r.findSiaObject(ctx, bucket, fileName)
	if err != nil {
		return nil, err
	}
	if siaObj == nil {
		return nil, fmt.Errorf("object not found: %s/%s", bucket, fileName)
	}

	// If staged or packing, read from staging backend.
	if siaObj.Status == models.RenterObjectStatusStaged || siaObj.Status == models.RenterObjectStatusPacking {
		offset := int64(0)
		length := int64(-1) // -1 means read all
		if options.Range != nil {
			offset = options.Range.Offset
			if options.Range.Length > 0 {
				length = options.Range.Length
			}
		}
		return r.stagingBackend.Get(ctx, siaObj.StagingKey, offset, length)
	}

	// If uploaded, unseal and download from SDK.
	if siaObj.SiaObjectID == "" {
		return nil, fmt.Errorf("object has no sia_object_id and no staging_key: %s/%s", bucket, fileName)
	}

	var sealed sdk.SealedObject
	if err := json.Unmarshal(siaObj.SealedData, &sealed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sealed object: %w", err)
	}

	obj, err := r.sdk.UnsealObject(sealed)
	if err != nil {
		return nil, fmt.Errorf("failed to unseal object: %w", err)
	}

	var dlOpts []sdk.DownloadOption
	if options.Range != nil && options.Range.Length > 0 {
		dlOpts = append(dlOpts, sdk.WithDownloadRange(uint64(options.Range.Offset), uint64(options.Range.Length)))
	} else if options.Range != nil && options.Range.Offset > 0 {
		dlOpts = append(dlOpts, sdk.WithDownloadRange(uint64(options.Range.Offset), 0))
	}

	return r.sdk.Download(ctx, obj, dlOpts...)
}

// GetObjectMetadata returns metadata for an object. This is a local DB query
// -- no network call is made.
func (r *RenterService) GetObjectMetadata(ctx context.Context, bucket string, fileName string) (*core.ObjectMetadata, error) {
	ctx, span := core.TraceMethod(ctx, "RenterService.GetObjectMetadata")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
	)
	if err := r.ensureSDK(); err != nil {
		return nil, err
	}
	siaObj, err := r.findSiaObject(ctx, bucket, fileName)
	if err != nil {
		return nil, err
	}
	if siaObj == nil {
		return nil, gorm.ErrRecordNotFound
	}

	return &core.ObjectMetadata{
		Bucket:  siaObj.Bucket,
		Key:     siaObj.ObjectKey,
		Size:    siaObj.Size,
		ModTime: siaObj.UpdatedAt,
	}, nil
}

// DeleteObject deletes an object from Sia or the staging backend, and removes
// the DB row. Uses CAS on the Status field to prevent races with the packing
// loop: if the object is in "packing" state, the delete fails because the
// packing loop is actively uploading it. The packing loop will detect the
// deletion via its own CAS (packing->uploaded) failing and clean up the
// orphaned SDK object.
func (r *RenterService) DeleteObject(ctx context.Context, bucket string, fileName string) error {
	ctx, span := core.TraceMethod(ctx, "RenterService.DeleteObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
	)
	if err := r.ensureSDK(); err != nil {
		return err
	}
	siaObj, err := r.findSiaObject(ctx, bucket, fileName)
	if err != nil {
		return err
	}
	if siaObj == nil {
		return nil // already deleted
	}

	// Parse the Sia object ID before transitioning state, so a parse
	// failure does not leave the object stuck in "deleting".
	var siaObjID [32]byte
	if siaObj.SiaObjectID != "" {
		siaObjID, err = parseSiaObjectID(siaObj.SiaObjectID)
		if err != nil {
			return fmt.Errorf("failed to parse sia_object_id: %w", err)
		}
	}

	// FSM CAS transition: current state -> deleting.
	// The FSM validates that the current state is a legal source for the
	// "delete" event (staged or uploaded). If the object is "packing", the
	// FSM rejects the transition -- the packing loop is actively uploading it
	// and will detect the deletion via its own CAS (packing->uploaded) failing.
	rowsAffected, err := indexd.TransitionState(r, ctx, siaObj.ID, siaObj.Status, models.RenterObjectStatusDeleting)
	if err != nil {
		return fmt.Errorf("FSM transition %s->deleting failed: %w", siaObj.Status, err)
	}

	if rowsAffected == 0 {
		// Object state changed between read and CAS -- another goroutine
		// (e.g. packing loop) modified it.
		return fmt.Errorf("cannot delete object %s/%s: state changed during delete (was %s)", bucket, fileName, siaObj.Status)
	}

	// Delete the DB row first so that if it fails, the backing data
	// (SDK object, staging data) is still intact and the row is not
	// stuck in "deleting" with no backing data.
	if err := db.RetryableComponentLock(r, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Unscoped().Delete(siaObj)
	}); err != nil {
		return fmt.Errorf("failed to delete sia_object record: %w", err)
	}

	// If uploaded, delete from SDK. siaObjID was parsed before the FSM
	// transition, so we can use it directly.
	if siaObj.SiaObjectID != "" {
		if err := r.sdk.DeleteObject(ctx, siaObjID); err != nil {
			r.Logger().Warn("failed to delete object from SDK",
				zap.String("siaObjectID", siaObj.SiaObjectID),
				zap.Error(err),
			)
		}
	}

	// If staged, delete from staging backend.
	if siaObj.StagingKey != "" {
		if err := r.stagingBackend.Delete(ctx, siaObj.StagingKey); err != nil {
			r.Logger().Warn("failed to delete staged object",
				zap.String("stagingKey", siaObj.StagingKey),
				zap.Error(err),
			)
		}
	}

	return nil
}

// DeleteObjectMetadata is the same as DeleteObject -- the SDK manages metadata
// internally, so we just delete the object and DB row.
func (r *RenterService) DeleteObjectMetadata(ctx context.Context, bucket string, fileName string) error {
	return r.DeleteObject(ctx, bucket, fileName)
}

// UploadExists checks if an object already exists in the database.
func (r *RenterService) UploadExists(ctx context.Context, bucket string, fileName string) (bool, *models.RenterObject, error) {
	ctx, span := core.TraceMethod(ctx, "RenterService.UploadExists")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.bucket", bucket),
		attribute.String("renter.objectKey", fileName),
	)
	if err := r.ensureSDK(); err != nil {
		return false, nil, err
	}
	siaObj, err := r.findSiaObject(ctx, bucket, fileName)
	if err != nil {
		return false, nil, err
	}
	return siaObj != nil, siaObj, nil
}

// SlabSize returns the slab size threshold for the packing loop.
func (r *RenterService) SlabSize(ctx context.Context) (uint64, error) {
	if err := r.ensureSDK(); err != nil {
		return 0, err
	}
	return uint64(r.slabSize), nil
}

// Stop shuts down the packing loop and waits for it to exit.
func (r *RenterService) Stop() error {
	indexd.StopPackingLoop(r.packingLoopCancel, r.packingLoopDone, r.Logger())
	if r.syncLoopCancel != nil {
		r.syncLoopCancel()
		if r.syncLoopDone != nil {
			select {
			case <-r.syncLoopDone:
			case <-time.After(10 * time.Second):
				r.Logger().Warn("sealed-data sync loop did not stop within timeout")
			}
		}
	}
	return nil
}

// findSiaObject queries the database for a RenterObject by protocol and object key.
// The unique index idx_renter_object_key is on (Protocol, ObjectKey).
func (r *RenterService) findSiaObject(ctx context.Context, protocol string, fileName string) (*models.RenterObject, error) {
	ctx, span := core.TraceMethod(ctx, "RenterService.findSiaObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("renter.protocol", protocol),
		attribute.String("renter.objectKey", fileName),
	)
	var siaObj models.RenterObject
	siaObj.Protocol = protocol
	siaObj.ObjectKey = fileName

	if err := db.RetryableComponentLock(r, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.RenterObject{}).Where(&siaObj).First(&siaObj)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &siaObj, nil
}

// parseSiaObjectID parses a hex-encoded types.Hash256 string.
func parseSiaObjectID(idHex string) ([32]byte, error) {
	var id [32]byte
	n, err := hex.Decode(id[:], []byte(idHex))
	if err != nil {
		return id, fmt.Errorf("invalid hex: %w", err)
	}
	if n != 32 {
		return id, fmt.Errorf("expected 32 bytes, got %d", n)
	}
	return id, nil
}
