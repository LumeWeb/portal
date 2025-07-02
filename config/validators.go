package config

import (
	"fmt"
	"strconv"
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

func makeUintCoercer[T ~uint | ~uint64](parseFn func(any) (T, error)) func(any) (any, error) {
	return func(data any) (any, error) {
		num, err := parseFn(data)
		if err != nil {
			return nil, err
		}
		return num, nil
	}
}

// ZogUInt creates a NumberSchema for uint values
func ZogUInt(opts ...zog.SchemaOption) *zog.NumberSchema[uint] {
	s := &zog.NumberSchema[uint]{}
	parseFn := func(data any) (uint, error) {
		num, err := conf.Coercers.Int(data)
		if err != nil {
			return 0, err
		}
		if num.(int) < 0 {
			return 0, fmt.Errorf("value must be non-negative, got %d", num.(int))
		}
		return uint(num.(int)), nil
	}
	opts = append(opts, zog.WithCoercer(makeUintCoercer(parseFn)))
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ZogUInt64 creates a NumberSchema for uint64 values
func ZogUInt64(opts ...zog.SchemaOption) *zog.NumberSchema[uint64] {
	s := &zog.NumberSchema[uint64]{}
	parseFn := func(data any) (uint64, error) {
		str := fmt.Sprintf("%v", data)
		return strconv.ParseUint(str, 10, 64)
	}
	opts = append(opts, zog.WithCoercer(makeUintCoercer(parseFn)))
	for _, opt := range opts {
		opt(s)
	}
	return s
}
