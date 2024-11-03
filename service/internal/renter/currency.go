package renter

import (
	"errors"
	"go.sia.tech/core/types"
	"math/big"
)

func newRat(num string, name string) (*big.Rat, error) {
	parsedNum, ok := new(big.Rat).SetString(num)

	if !ok {
		return nil, errors.New("failed to parse " + name)
	}

	return parsedNum, nil
}

func siacoinsFromRat(r *big.Rat) (types.Currency, error) {
	r.Mul(r, new(big.Rat).SetInt(types.HastingsPerSiacoin.Big()))
	i := new(big.Int).Div(r.Num(), r.Denom())
	if i.Sign() < 0 {
		return types.ZeroCurrency, errors.New("value cannot be negative")
	} else if i.BitLen() > 128 {
		return types.ZeroCurrency, errors.New("value overflows Currency representation")
	}
	return types.NewCurrency(i.Uint64(), new(big.Int).Rsh(i, 64).Uint64()), nil
}

func siacoinsToRat(c types.Currency) *big.Rat {
	// Convert Currency to big.Int hastings
	hastings := c.Big()

	// Convert to siacoins by dividing by HastingsPerSiacoin
	return new(big.Rat).Quo(
		new(big.Rat).SetInt(hastings),
		new(big.Rat).SetInt(types.HastingsPerSiacoin.Big()),
	)
}

func ratDivide(a *big.Rat, b uint64) *big.Rat {
	return new(big.Rat).Quo(a, new(big.Rat).SetUint64(b))
}
func ratMultiply(a *big.Rat, b uint64) *big.Rat {
	return new(big.Rat).Mul(a, new(big.Rat).SetUint64(b))
}
