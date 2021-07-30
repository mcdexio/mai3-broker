package match

import (
	"fmt"
	"testing"

	"github.com/mcdexio/mai3-broker/common/mai3"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

var defaultPool = &model.LiquidityPoolStorage{
	VaultFeeRate:    decimal.NewFromFloat(0.0002),
	PoolCashBalance: decimal.NewFromInt(0), // set me later

	Perpetuals: make(map[int64]*model.PerpetualStorage), // set me later
}

var OpenSlippageFactor, _ = decimal.NewFromString("0.0142857142857142857142857142857")
var CloseSlippageFactor, _ = decimal.NewFromString("0.0128571428571428571428571428571")

var perpetual1 = &model.PerpetualStorage{
	IsNormal: true,

	MarkPrice:               decimal.NewFromFloat(6965),
	IndexPrice:              decimal.NewFromFloat(7000),
	UnitAccumulativeFunding: decimal.NewFromFloat(9.9059375),

	InitialMarginRate:      decimal.NewFromFloat(0.1),
	MaintenanceMarginRate:  decimal.NewFromFloat(0.05),
	OperatorFeeRate:        decimal.NewFromFloat(0.0001),
	LpFeeRate:              decimal.NewFromFloat(0.0007),
	ReferrerRebateRate:     decimal.NewFromFloat(0),
	LiquidationPenaltyRate: decimal.NewFromFloat(0.005),
	KeeperGasReward:        decimal.NewFromFloat(1),
	InsuranceFundRate:      decimal.NewFromFloat(0.0001),
	OpenInterest:           decimal.NewFromFloat(10),
	MaxOpenInterestRate:    decimal.NewFromFloat(100),

	HalfSpread:            decimal.NewFromFloat(0.001),
	OpenSlippageFactor:    OpenSlippageFactor,
	CloseSlippageFactor:   CloseSlippageFactor,
	FundingRateLimit:      decimal.NewFromFloat(0.005),
	MaxLeverage:           decimal.NewFromFloat(5),
	MaxClosePriceDiscount: decimal.NewFromFloat(0.05),

	AmmCashBalance:    decimal.NewFromInt(0), // assign me later
	AmmPositionAmount: decimal.NewFromInt(0), // assign me later
}

var accountStorage1 = &model.AccountStorage{
	CashBalance:    decimal.NewFromFloat(7698.86),
	PositionAmount: decimal.NewFromFloat(2.3),
	TargetLeverage: decimal.NewFromFloat(2),
}

var accountStorage0 = &model.AccountStorage{
	CashBalance:    decimal.NewFromFloat(1000),
	PositionAmount: decimal.NewFromFloat(0),
	TargetLeverage: decimal.NewFromFloat(2),
}

const TEST_MARKET_INDEX0 = 0

func Approximate(t *testing.T, expect, actual decimal.Decimal, msgAndArgs ...interface{}) {
	if expect.Sub(actual).Abs().GreaterThan(decimal.New(1, -12)) {
		assert.Fail(t, fmt.Sprintf("approximate: expect[%s] actual[%s]", expect, actual), msgAndArgs...)
	}
}

// empty order book. close only
func TestComputeOrderAvailable1(t *testing.T) {
	order := &model.Order{
		AvailableAmount: decimal.NewFromFloat(-1),
	}
	order.Price = decimal.NewFromFloat(6900)
	poolStorage := defaultPool
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	poolStorage.Perpetuals[TEST_MARKET_INDEX0] = perpetual1

	_, available := ComputeOrderAvailable(poolStorage, TEST_MARKET_INDEX0, accountStorage1, []*model.Order{order})
	Approximate(t, decimal.NewFromFloat(0), available)
}

// empty order book. close + open. withdraw covers deposit
func TestComputeOrderAvailable2(t *testing.T) {
	order := &model.Order{
		CheckAmount: decimal.NewFromFloat(-3.3),
	}
	order.Price = decimal.NewFromFloat(6900)
	poolStorage := defaultPool
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	poolStorage.Perpetuals[TEST_MARKET_INDEX0] = perpetual1
	accountStorage1.WalletBalance = _0

	// marginBalance = 23695.57634375
	// | loss | = (6965 - 6900) * 2.3 = 149.5, withdraw = marginBalance - | loss | - closeFee = 23530.20634375
	// | loss | = (6965 - 6900) * 1 = 65, deposit = 6965 * 1 / lev + | loss | + openFee = 3554.4
	computeAccount, _ := mai3.ComputeAccount(poolStorage, TEST_MARKET_INDEX0, accountStorage1)
	Approximate(t, decimal.NewFromFloat(23695.57634375), computeAccount.MarginBalance)

	_, remainWalletBalance := sideAvailable(poolStorage, TEST_MARKET_INDEX0, computeAccount.MarginBalance, accountStorage1.PositionAmount, accountStorage1.TargetLeverage, accountStorage1.WalletBalance, []*model.Order{order})
	Approximate(t, decimal.NewFromFloat(19974.80634375), remainWalletBalance)
}

// empty order book. close + open
func TestComputeOrderAvailable3(t *testing.T) {
	order := &model.Order{
		AvailableAmount: decimal.NewFromFloat(-10),
	}
	order.Price = decimal.NewFromFloat(6900)

	poolStorage := defaultPool
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	poolStorage.Perpetuals[TEST_MARKET_INDEX0] = perpetual1
	accountStorage1.WalletBalance = _0

	// marginBalance = 23695.57634375
	// withdraw = 23695.57634375 - (6965 - 6900)*2.3 - 6900*2.3*0.001 = 23530.20634375
	// deposit = (10 - 2.3)*6965/2 + (10 - 2.3)*6900*0.001 + (6965 - 6900)*(10 - 2.3) = 27368.9
	// cost = deposit - withdraw = 3839.67365625
	_, available := ComputeOrderAvailable(poolStorage, TEST_MARKET_INDEX0, accountStorage1, []*model.Order{order})
	Approximate(t, decimal.NewFromFloat(-3839.67365625), available)
}

// empty order book. pos = 0 but cash > 0. open short. limit < mark
func TestComputeOrderAvailable4(t *testing.T) {
	order := &model.Order{
		AvailableAmount: decimal.NewFromFloat(-10),
	}
	order.Price = decimal.NewFromFloat(6900)

	poolStorage := defaultPool
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	poolStorage.Perpetuals[TEST_MARKET_INDEX0] = perpetual1

	// deposit = 10*6965/2 + 10*6900*0.001 + (6965 - 6900)*10 = 35545
	// cost = deposit - 1000 = 34545
	_, available := ComputeOrderAvailable(poolStorage, TEST_MARKET_INDEX0, accountStorage0, []*model.Order{order})
	Approximate(t, decimal.NewFromFloat(-34545), available)
}

// empty order book. pos = 0 but cash > 0. open short. limit > mark
func TestComputeOrderAvailable5(t *testing.T) {
	order := &model.Order{
		AvailableAmount: decimal.NewFromFloat(-10),
	}
	order.Price = decimal.NewFromFloat(7000)

	poolStorage := defaultPool
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	poolStorage.Perpetuals[TEST_MARKET_INDEX0] = perpetual1

	// deposit = 10*7000/2 + 10*7000*0.001 + 0(profit) = 35071
	// cost = deposit - 1000 = 34070

	_, available := ComputeOrderAvailable(poolStorage, TEST_MARKET_INDEX0, accountStorage0, []*model.Order{order})
	Approximate(t, decimal.NewFromFloat(-34071), available)
}

// empty order book. pos = 0 but cash > 0. open long. limit < mark
func TestComputeOrderAvailable6(t *testing.T) {
	order := &model.Order{
		AvailableAmount: decimal.NewFromFloat(10),
	}
	order.Price = decimal.NewFromFloat(6900)

	poolStorage := defaultPool
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	poolStorage.Perpetuals[TEST_MARKET_INDEX0] = perpetual1

	// deposit = 10*6900/2 + 10*6900*0.001 + 0(profit) = 33570
	// cost = deposit - 1000 = 33570
	_, available := ComputeOrderAvailable(poolStorage, TEST_MARKET_INDEX0, accountStorage0, []*model.Order{order})
	Approximate(t, decimal.NewFromFloat(-33570), available)
}

// empty order book. pos = 0 but cash > 0. open long. limit > mark
func TestComputeOrderAvailable7(t *testing.T) {
	order := &model.Order{
		AvailableAmount: decimal.NewFromFloat(10),
	}
	order.Price = decimal.NewFromFloat(7000)

	poolStorage := defaultPool
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	poolStorage.Perpetuals[TEST_MARKET_INDEX0] = perpetual1

	// deposit = 10*7000/2 + 10*7000*0.001 + (7000 - 6965)*10 = 35421
	// cost = deposit - 1000 = 34421
	_, available := ComputeOrderAvailable(poolStorage, TEST_MARKET_INDEX0, accountStorage0, []*model.Order{order})
	Approximate(t, decimal.NewFromFloat(-34421), available)
}
