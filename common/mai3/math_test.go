package mai3

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func mustNewDecimalFromString(s string) decimal.Decimal {
	if ret, err := decimal.NewFromString(s); err != nil {
		panic(err)
	} else {
		return ret
	}
}

func TestSqrt(t *testing.T) {
	assert.Equal(t, true, mustNewDecimalFromString("0").Equal(Sqrt(mustNewDecimalFromString("0"))))
	Approximate(t, mustNewDecimalFromString("0"), Sqrt(mustNewDecimalFromString("1e-36")))
	Approximate(t, mustNewDecimalFromString("1e-18"), Sqrt(mustNewDecimalFromString("2e-36")))
	Approximate(t, mustNewDecimalFromString("1e-18"), Sqrt(mustNewDecimalFromString("3e-36")))
	Approximate(t, mustNewDecimalFromString("2e-18"), Sqrt(mustNewDecimalFromString("4e-36")))
	Approximate(t, mustNewDecimalFromString("2e-18"), Sqrt(mustNewDecimalFromString("5e-36")))
	Approximate(t, mustNewDecimalFromString("2e-18"), Sqrt(mustNewDecimalFromString("6e-36")))
	Approximate(t, mustNewDecimalFromString("2e-18"), Sqrt(mustNewDecimalFromString("7e-36")))
	Approximate(t, mustNewDecimalFromString("2e-18"), Sqrt(mustNewDecimalFromString("8e-36")))
	Approximate(t, mustNewDecimalFromString("3e-18"), Sqrt(mustNewDecimalFromString("9e-36")))
	Approximate(t, mustNewDecimalFromString("1e-9"), Sqrt(mustNewDecimalFromString("1e-18")))
	assert.Equal(t, true, mustNewDecimalFromString("0.35").Equal(Sqrt(mustNewDecimalFromString("0.1225"))))
	assert.Equal(t, true, mustNewDecimalFromString("1").Equal(Sqrt(mustNewDecimalFromString("1"))))
	assert.Equal(t, true, mustNewDecimalFromString("5").Equal(Sqrt(mustNewDecimalFromString("25"))))
	assert.Equal(t, true, mustNewDecimalFromString("7e4").Equal(Sqrt(mustNewDecimalFromString("49e8"))))
	assert.Equal(t, true, mustNewDecimalFromString("1e12").Equal(Sqrt(mustNewDecimalFromString("1e24"))))
	Approximate(t, mustNewDecimalFromString("1.888560298216607115171146459645"), Sqrt(mustNewDecimalFromString("3.56666")))
	Approximate(t, mustNewDecimalFromString("1356432.176631032297040481823579"), Sqrt(mustNewDecimalFromString("1839908249800")))
	Approximate(t, mustNewDecimalFromString("170141183460469231731.687303715884105728"), Sqrt(mustNewDecimalFromString("28948022309329048855892746252171976963317.496166410141009864396001978282409984")))
	Approximate(t, mustNewDecimalFromString("240615969168004511545.033772477625056927"), Sqrt(mustNewDecimalFromString("57896044618658097711785492504343953926634.992332820282019728792003956564819967")))
}

func TestGss(t *testing.T) {
	f := func(x float64) float64 {
		tmp := x - 2
		return tmp * tmp
	}
	res := Gss(f, 1, 5, 1e-8, 15)
	assert.Less(t, 1.9987067953999975, res)
	assert.Greater(t, 2.001639345143427, res)
}
