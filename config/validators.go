package config

import (
	"github.com/Oudwins/zog"
	"github.com/Oudwins/zog/conf"
)

// StringLike creates a StringSchema for type aliases of string.
func ZogStringLike[T ~string](opts ...zog.SchemaOption) *zog.StringSchema[T] {
	s := &zog.StringSchema[T]{}

	// Custom coercer to handle the type alias conversion during coercion.
	customCoercer := func(data any) (any, error) {
		str, err := conf.Coercers.String(data)
		if err != nil {
			return nil, err
		}
		return T(str.(string)), nil
	}

	opts = append(opts, zog.WithCoercer(customCoercer))
	for _, opt := range opts {
		opt(s)
	}
	return s
}
