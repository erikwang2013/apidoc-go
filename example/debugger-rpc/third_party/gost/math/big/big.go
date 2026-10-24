// Package gxbig is the subset of github.com/dubbogo/gost/math/big that
// dubbo-go-hessian2 registers for java.math.BigInteger and java.math.BigDecimal.
// Primitive Hessian values never touch it; the types exist so the library's
// Big* serializers compile and round-trip their wire form.
package gxbig

import (
	"fmt"
	"math/big"
)

// Integer mirrors the wire fields of the upstream type.
type Integer struct {
	value  big.Int
	Signum int32
	Mag    []int
}

func (Integer) JavaClassName() string { return "java.math.BigInteger" }

func (i *Integer) FromString(s string) error {
	if _, ok := i.value.SetString(s, 10); !ok {
		return fmt.Errorf("invalid integer %q", s)
	}
	return nil
}

func (i *Integer) FromSignAndMag(signum int32, mag []int) { i.Signum, i.Mag = signum, mag }
func (i *Integer) GetSignAndMag() (int32, []int)          { return i.Signum, i.Mag }
func (i *Integer) String() string                         { return i.value.String() }

// Decimal mirrors the upstream wire field.
type Decimal struct{ Value string }

func (Decimal) JavaClassName() string { return "java.math.BigDecimal" }
func (d *Decimal) String() string     { return d.Value }

func (d *Decimal) FromString(s string) error {
	d.Value = s
	return nil
}
