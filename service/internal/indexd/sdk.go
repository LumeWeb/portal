package indexd

import (
	"context"
	"io"
	"sync"
	"time"

	"go.lumeweb.com/portal/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
	sdk "go.sia.tech/siastorage"
)

// UploadedObject abstracts a finalized SDK object so the packing loop and tests
// can work with mock objects without depending on concrete sdk.Object.
type UploadedObject interface {
	// ID returns the object's unique identifier.
	ID() types.Hash256
}

// PackedUploader abstracts the SDK's PackedUpload handle. It allows multiple
// objects to be added to a single packed upload, then finalized.
type PackedUploader interface {
	// Add adds an object's data to the packed upload. Returns the number of
	// bytes written.
	Add(ctx context.Context, r io.Reader) (int64, error)
	// Finalize completes the upload and returns the resulting objects.
	Finalize(ctx context.Context) ([]sdk.Object, error)
	// Close releases resources. Safe to call multiple times.
	Close() error
}

// SDK is the subset of the siastorage SDK that the indexd renter service uses.
// Defined as an interface so the service can be tested with a mock.
type SDK interface {
	// UploadPacked creates a new packed upload handle.
	UploadPacked(ctx context.Context, opts ...sdk.UploadOption) (PackedUploader, error)
	// Download retrieves an object's data.
	Download(ctx context.Context, obj sdk.Object, opts ...sdk.DownloadOption) (io.ReadCloser, error)
	// PinObject pins an object so it persists.
	PinObject(ctx context.Context, obj sdk.Object) error
	// DeleteObject deletes an object by its ID.
	DeleteObject(ctx context.Context, key [32]byte) error
	// SealObject seals an object for later reconstruction.
	SealObject(obj sdk.Object) sdk.SealedObject
	// UnsealObject reconstructs an object from its sealed form.
	UnsealObject(sealed sdk.SealedObject) (sdk.Object, error)
	// ObjectEvents returns object events (create/update/delete) from the
	// indexer, starting from the given cursor. Used by the sealed-data
	// refresh loop to keep local sealed objects in sync with the indexer.
	ObjectEvents(ctx context.Context, cursor slabs.Cursor, limit int) ([]sdk.ObjectEvent, error)
}

// sdkPackedUpload adapts *sdk.PackedUpload to the PackedUploader interface.
type sdkPackedUpload struct {
	inner *sdk.PackedUpload
}

func (u *sdkPackedUpload) Add(ctx context.Context, r io.Reader) (int64, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.sdkPackedUpload.Add")
	defer span.End()
	n, err := u.inner.Add(ctx, r)
	if err == nil {
		span.SetAttributes(attribute.Int64("indexd.packed.bytesWritten", n))
	}
	return n, err
}

func (u *sdkPackedUpload) Finalize(ctx context.Context) ([]sdk.Object, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.sdkPackedUpload.Finalize")
	defer span.End()
	results, err := u.inner.Finalize(ctx)
	if err == nil {
		span.SetAttributes(attribute.Int("indexd.packed.objectCount", len(results)))
	}
	return results, err
}

func (u *sdkPackedUpload) Close() error {
	return u.inner.Close()
}

// SDKAdapter wraps a concrete *sdk.SDK to satisfy the SDK interface.
// This is the production implementation — tests use mock SDKs directly.
type SDKAdapter struct {
	Inner  *sdk.SDK
	AppKey types.PrivateKey
}

func (a *SDKAdapter) UploadPacked(ctx context.Context, opts ...sdk.UploadOption) (PackedUploader, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.SDKAdapter.UploadPacked")
	defer span.End()
	pu, err := a.Inner.UploadPacked(opts...)
	if err == nil {
		span.SetAttributes(attribute.Int("indexd.uploadOpts", len(opts)))
	}
	return &sdkPackedUpload{inner: pu}, err
}

func (a *SDKAdapter) Download(ctx context.Context, obj sdk.Object, opts ...sdk.DownloadOption) (io.ReadCloser, error) {
	ctx, span := core.TraceMethod(ctx, "indexd.SDKAdapter.Download")
	defer span.End()
	span.SetAttributes(
		attribute.String("indexd.siaObjectID", obj.ID().String()),
		attribute.Int("indexd.downloadOpts", len(opts)),
	)
	rc, err := a.Inner.Download(obj, opts...)
	if err != nil {
		return nil, err
	}
	return &tracedReadCloser{
		inner: rc,
		ctx:   ctx,
		objID: obj.ID().String(),
	}, nil
}

func (a *SDKAdapter) PinObject(ctx context.Context, obj sdk.Object) error {
	ctx, span := core.TraceMethod(ctx, "indexd.SDKAdapter.PinObject")
	defer span.End()
	span.SetAttributes(attribute.String("indexd.siaObjectID", obj.ID().String()))
	return a.Inner.PinObject(ctx, obj)
}

func (a *SDKAdapter) DeleteObject(ctx context.Context, key [32]byte) error {
	ctx, span := core.TraceMethod(ctx, "indexd.SDKAdapter.DeleteObject")
	defer span.End()
	span.SetAttributes(attribute.String("indexd.siaObjectID", types.Hash256(key).String()))
	return a.Inner.DeleteObject(ctx, key)
}

func (a *SDKAdapter) SealObject(obj sdk.Object) sdk.SealedObject {
	return obj.Seal(a.AppKey)
}

func (a *SDKAdapter) UnsealObject(sealed sdk.SealedObject) (sdk.Object, error) {
	return sealed.Open(a.AppKey)
}

// tracedReadCloser wraps an io.ReadCloser from the Sia SDK and creates a span
// around the actual network I/O — the time between the first Read and Close.
// The SDK's Download method returns a lazy reader (pipe + goroutine), so the
// real download happens on Read, not on the Download call itself. This wrapper
// measures that boundary without buffering or copying data.
type tracedReadCloser struct {
	inner      io.ReadCloser
	ctx        context.Context
	objID      string
	once       sync.Once
	span       trace.Span
	totalBytes int64
	startTime  time.Time
}

func (t *tracedReadCloser) Read(p []byte) (int, error) {
	t.once.Do(func() {
		_, t.span = core.TraceMethod(t.ctx, "indexd.SDKAdapter.Download.Read")
		t.span.SetAttributes(attribute.String("indexd.siaObjectID", t.objID))
		t.startTime = time.Now()
		DownloadActive.WithLabelValues(LabelOpDownload).Inc()
	})
	n, err := t.inner.Read(p)
	t.totalBytes += int64(n)
	if err != nil && err != io.EOF {
		t.span.RecordError(err)
		t.span.SetStatus(codes.Error, err.Error())
		DownloadErrors.WithLabelValues(LabelOpDownload).Inc()
	}
	return n, err
}

func (t *tracedReadCloser) Close() error {
	t.once.Do(func() {
		// If Read was never called, still create the span so it shows up
		// as a zero-byte read rather than vanishing entirely.
		_, t.span = core.TraceMethod(t.ctx, "indexd.SDKAdapter.Download.Read")
		t.startTime = time.Now()
	})
	t.span.SetAttributes(attribute.Int64("indexd.downloadBytes", t.totalBytes))
	t.span.End()

	// Record Prometheus metrics for the network I/O boundary.
	if !t.startTime.IsZero() {
		DownloadDuration.WithLabelValues(LabelOpDownload).
			Observe(time.Since(t.startTime).Seconds())
		DownloadBytes.WithLabelValues(LabelOpDownload).
			Add(float64(t.totalBytes))
		DownloadActive.WithLabelValues(LabelOpDownload).Dec()
	}
	return t.inner.Close()
}

func (a *SDKAdapter) ObjectEvents(ctx context.Context, cursor slabs.Cursor, limit int) ([]sdk.ObjectEvent, error) {
	return a.Inner.ObjectEvents(ctx, cursor, limit)
}
