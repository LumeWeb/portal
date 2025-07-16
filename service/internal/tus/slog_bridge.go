package tus

import (
	"context"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap/exp/zapslog"
	"golang.org/x/exp/slog"
	newSlog "log/slog"
)

// SlogBridge bridges between the old and new slog versions
type SlogBridge struct {
	logger *newSlog.Logger // new slog.Logger
}

// NewSlogBridge creates a new bridge between slog versions
func NewSlogBridge(logger *core.Logger) *SlogBridge {
	// Convert zap logger to new slog.Logger
	newSlogger := newSlog.New(zapslog.NewHandler(logger.Core()))
	return &SlogBridge{
		logger: newSlogger,
	}
}

// Handler returns a handler compatible with the old slog version
func (b *SlogBridge) Handler() slog.Handler {
	return &handlerBridge{newHandler: b.logger.Handler()}
}

func convertAttr(attr slog.Attr) newSlog.Attr {
	switch attr.Value.Kind() {
	case slog.KindBool:
		return newSlog.Bool(attr.Key, attr.Value.Bool())
	case slog.KindDuration:
		return newSlog.Duration(attr.Key, attr.Value.Duration())
	case slog.KindFloat64:
		return newSlog.Float64(attr.Key, attr.Value.Float64())
	case slog.KindInt64:
		return newSlog.Int64(attr.Key, attr.Value.Int64())
	case slog.KindString:
		return newSlog.String(attr.Key, attr.Value.String())
	case slog.KindTime:
		return newSlog.Time(attr.Key, attr.Value.Time())
	case slog.KindUint64:
		return newSlog.Uint64(attr.Key, attr.Value.Uint64())
	case slog.KindGroup:
		group := attr.Value.Group()
		attrs := make([]any, len(group))
		for i, a := range group {
			attrs[i] = convertAttr(a)
		}
		return newSlog.Group(attr.Key, attrs...)
	case slog.KindLogValuer:
		return convertAttr(slog.Attr{
			Key:   attr.Key,
			Value: attr.Value.Resolve(),
		})
	default:
		return newSlog.Any(attr.Key, attr.Value.Any())
	}
}

// handlerBridge implements the old slog.Handler interface using the new slog.Handler
type handlerBridge struct {
	newHandler newSlog.Handler
}

func (h *handlerBridge) Enabled(ctx context.Context, level slog.Level) bool {
	return h.newHandler.Enabled(ctx, newSlog.Level(level))
}

func (h *handlerBridge) Handle(ctx context.Context, r slog.Record) error {
	// Convert old Record to new Record
	newRecord := newSlog.NewRecord(
		r.Time,
		newSlog.Level(r.Level),
		r.Message,
		r.PC,
	)
	r.Attrs(func(attr slog.Attr) bool {
		newRecord.Add(convertAttr(attr))
		return true
	})
	return h.newHandler.Handle(ctx, newRecord)
}

func (h *handlerBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]newSlog.Attr, len(attrs))
	for i, attr := range attrs {
		newAttrs[i] = convertAttr(attr)
	}
	return &handlerBridge{newHandler: h.newHandler.WithAttrs(newAttrs)}
}

func (h *handlerBridge) WithGroup(name string) slog.Handler {
	return &handlerBridge{newHandler: h.newHandler.WithGroup(name)}
}
