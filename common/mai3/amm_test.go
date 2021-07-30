package mai3

import (
	"fmt"
	"testing"

	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func Approximate(t *testing.T, expect, actual decimal.Decimal, msgAndArgs ...interface{}) {
	if expect.Sub(actual).Abs().GreaterThan(decimal.New(1, -10)) {
		assert.Fail(t, fmt.Sprintf("approximate: expect[%s] actual[%s]", expect, actual), msgAndArgs...)
	}
}

var defaultPool = &model.LiquidityPoolStorage{
	VaultFeeRate:    decimal.NewFromFloat(0.0002),
	PoolCashBalance: _0, // set me later

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

	AmmCashBalance:    _0, // assign me later
	AmmPositionAmount: _0, // assign me later
}

var perpetual0 = &model.PerpetualStorage{
	IsNormal: true,

	MarkPrice:               decimal.NewFromFloat(95),
	IndexPrice:              decimal.NewFromFloat(100),
	UnitAccumulativeFunding: decimal.NewFromFloat(1.9),

	InitialMarginRate:      decimal.NewFromFloat(0.1),
	MaintenanceMarginRate:  decimal.NewFromFloat(0.05),
	OperatorFeeRate:        decimal.NewFromFloat(0.0001),
	LpFeeRate:              decimal.NewFromFloat(0.0008),
	ReferrerRebateRate:     decimal.NewFromFloat(0.0000),
	LiquidationPenaltyRate: decimal.NewFromFloat(0.005),
	KeeperGasReward:        decimal.NewFromFloat(2),
	InsuranceFundRate:      decimal.NewFromFloat(0.0001),

	HalfSpread:            decimal.NewFromFloat(0.001),
	OpenSlippageFactor:    decimal.NewFromFloat(1),
	CloseSlippageFactor:   decimal.NewFromFloat(0.9),
	FundingRateLimit:      decimal.NewFromFloat(0.005),
	MaxLeverage:           decimal.NewFromFloat(3),
	MaxClosePriceDiscount: decimal.NewFromFloat(0.2),

	AmmCashBalance:    _0, // assign me later
	AmmPositionAmount: _0, // assign me later
}

var perpetual2 = &model.PerpetualStorage{
	IsNormal: true,

	MarkPrice:               decimal.NewFromFloat(95),
	IndexPrice:              decimal.NewFromFloat(100),
	UnitAccumulativeFunding: decimal.NewFromFloat(1.9),

	InitialMarginRate:      decimal.NewFromFloat(0.1),
	MaintenanceMarginRate:  decimal.NewFromFloat(0.05),
	OperatorFeeRate:        decimal.NewFromFloat(0.0001),
	LpFeeRate:              decimal.NewFromFloat(0.0008),
	ReferrerRebateRate:     decimal.NewFromFloat(0.0000),
	LiquidationPenaltyRate: decimal.NewFromFloat(0.005),
	KeeperGasReward:        decimal.NewFromFloat(2),
	InsuranceFundRate:      decimal.NewFromFloat(0.0001),

	HalfSpread:            decimal.NewFromFloat(0.001),
	OpenSlippageFactor:    decimal.NewFromFloat(1),
	CloseSlippageFactor:   decimal.NewFromFloat(0.9),
	FundingRateLimit:      decimal.NewFromFloat(0.005),
	MaxLeverage:           decimal.NewFromFloat(3),
	MaxClosePriceDiscount: decimal.NewFromFloat(0.2),

	AmmCashBalance:    _0, // assign me later
	AmmPositionAmount: _0, // assign me later
}

var accountStorage1 = &model.AccountStorage{
	CashBalance:    decimal.NewFromFloat(7698.86),
	PositionAmount: decimal.NewFromFloat(2.3),
	TargetLeverage: _1,
}

const TEST_PERPETUAL_INDEX0 = 0
const TEST_PERPETUAL_INDEX1 = 1

var poolStorage0 = &model.LiquidityPoolStorage{
	VaultFeeRate:    decimal.NewFromFloat(0.0002),
	PoolCashBalance: decimal.NewFromFloat(100000),

	Perpetuals: make(map[int64]*model.PerpetualStorage), // set me later
}

// amm holds short, trader buys
// case 1: amm unsafe
func TestComputeAMMAmountWithPrice1(t *testing.T) {
	limitPrice := decimal.NewFromFloat(100000)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(17096.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds short, trader buys
// case 2: lower than spread
func TestComputeAMMAmountWithPrice2(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7023.1160999)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds short, trader buys
// case 3: exactly the best ask/bid price
func TestComputeAMMAmountWithPrice3(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7023.1161)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(2.0046), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	assert.Equal(t, true, tradingPrice.LessThanOrEqual(limitPrice))
}

// amm holds short, trader buys
// case 4: limitPrice is far away
func TestComputeAMMAmountWithPrice4(t *testing.T) {
	limitPrice := decimal.NewFromFloat(100000)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(90.795503235030246126178607648), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	assert.Equal(t, true, tradingPrice.LessThan(limitPrice))
}

// amm holds short, trader buys
// case 5: normal
func TestComputeAMMAmountWithPrice5(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7200)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	// perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(52.542857142857142857), amount)
	accountStorage1.WalletBalance = decimal.NewFromFloat(378686.8799999999999989704)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds short, trader sells
// case 6: higher than spread
func TestComputeAMMAmountWithPrice6(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7000.001)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(17096.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds short, trader sells
// case 7:amm unsafe - exactly the best ask/bid price - close + open
func TestComputeAMMAmountWithPrice7(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7000)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(17096.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-2.3), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	assert.Equal(t, true, tradingPrice.LessThanOrEqual(limitPrice))
}

// amm holds short, trader sells
// case 8:amm unsafe - largest amount
func TestComputeAMMAmountWithPrice8(t *testing.T) {
	limitPrice := decimal.NewFromFloat(0)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(17096.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-3.248643177964958208), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	assert.Equal(t, true, tradingPrice.GreaterThanOrEqual(limitPrice))
}

// amm holds short, trader sells
// case 9:amm unsafe close + open
func TestComputeAMMAmountWithPrice9(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6992)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(17096.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-2.54339106672133532007243536012), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds short, trader sells
// case 10: higher than spread
func TestComputeAMMAmountWithPrice10(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7007.476)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds short, trader sells
// case 11:safe - exactly the best ask/bid price - close + open
func TestComputeAMMAmountWithPrice11(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7007.4752419462290525818804101137)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-2.226863373523786822), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds short, trader sells
// case 12:close only
func TestComputeAMMAmountWithPrice12(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7007.4)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-2.250750147989139645), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds short, trader sells
// case 13:close + open
func TestComputeAMMAmountWithPrice13(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7006)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(116095.73134375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-2.688951590780905289), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds long, trader buys
// case 14: amm unsafe - lower than spread
func TestComputeAMMAmountWithPrice14(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6999.999)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(-13677.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds long, trader buys
// case 15: amm unsafe - exactly the best ask/bid price - close + open
func TestComputeAMMAmountWithPrice15(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7000)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(-13677.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(2.3), amount)
	accountStorage1.WalletBalance = decimal.NewFromFloat(16116.1)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds long, trader buys
// case 16: amm unsafe - largest amount
func TestComputeAMMAmountWithPrice16(t *testing.T) {
	limitPrice := decimal.NewFromFloat(100000)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(-13677.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(4.534292077640725907), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	assert.Equal(t, true, tradingPrice.LessThanOrEqual(limitPrice))
}

// amm holds long, trader buys
// case 17: amm unsafe - close + open
func TestComputeAMMAmountWithPrice17(t *testing.T) {
	limitPrice := decimal.NewFromFloat(7008)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(-13677.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(2.68369217482083603940884140606), amount)
	accountStorage1.WalletBalance = decimal.NewFromFloat(18827)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds long, trader buys
// case 18: lower than spread
func TestComputeAMMAmountWithPrice18(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6992.495)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds long, trader buys
// case 19: exactly the best ask/bid price - close + open
func TestComputeAMMAmountWithPrice19(t *testing.T) {
	limitPrice, _ := decimal.NewFromString("6992.4957785904151334990367462224")
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(2.217663373523786822), amount)
	accountStorage1.WalletBalance = decimal.NewFromFloat(15523)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds long, trader buys
// case 20: close only
func TestComputeAMMAmountWithPrice20(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6992.7)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(2.282496767610908028), amount)
	accountStorage1.WalletBalance = decimal.NewFromFloat(15977)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds long, trader buys
// case 21: close + open
func TestComputeAMMAmountWithPrice21(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6994)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, true, limitPrice)
	Approximate(t, decimal.NewFromFloat(2.688951590780905289), amount)
	accountStorage1.WalletBalance = decimal.NewFromFloat(18826)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// amm holds long, trader sells
// case 22: amm unsafe
func TestComputeAMMAmountWithPrice22(t *testing.T) {
	limitPrice := decimal.NewFromFloat(0)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(-13677.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds long, trader sells
// case 23: higher than index
func TestComputeAMMAmountWithPrice23(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6976.9161001)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	assert.Equal(t, _0, amount)
}

// amm holds long, trader sells
// case 24: exactly the best ask/bid price
func TestComputeAMMAmountWithPrice24(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6976.9161)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-1.9954), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	assert.Equal(t, tradingPrice.LessThanOrEqual(limitPrice), true)
}

// amm holds long, trader sells
// case 25: limitPrice is far away
func TestComputeAMMAmountWithPrice25(t *testing.T) {
	limitPrice := decimal.NewFromFloat(0)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-90.795503235030246126178607648), amount)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	assert.Equal(t, tradingPrice.GreaterThanOrEqual(limitPrice), true)
}

// amm holds long, trader sells
// case 26: normal
func TestComputeAMMAmountWithPrice26(t *testing.T) {
	limitPrice := decimal.NewFromFloat(6800)
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	perpetual1.OpenInterest = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, decimal.NewFromFloat(-52.542857142857142857), amount)
	accountStorage1.WalletBalance = decimal.NewFromFloat(335273)
	_, _, tradingPrice, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, amount)
	Approximate(t, limitPrice, tradingPrice)
}

// open 0 -> -x
func TestComputeBestAskBidPrice0(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(10000)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(0)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual0

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, false)
	Approximate(t, bestPrice, decimal.NewFromFloat(100.1))
}

// open -10
func TestComputeBestAskBidPrice1(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(10100)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(-10)
	perpetual2.AmmPositionAmount = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual2

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, false)
	Approximate(t, decimal.NewFromFloat(110.11), bestPrice)
}

// open 0 -> +x
func TestComputeBestAskBidPrice2(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(10000)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(0)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual0

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, true)
	Approximate(t, decimal.NewFromFloat(99.9), bestPrice)
}

// open 10
func TestComputeBestAskBidPrice3(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(8138)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(10)
	perpetual2.AmmPositionAmount = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual2

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, true)
	Approximate(t, decimal.NewFromFloat(89.91), bestPrice)
}

// close -10
func TestComputeBestAskBidPrice4(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(10100)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(-10)
	perpetual2.AmmPositionAmount = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual2

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, true)
	Approximate(t, decimal.NewFromFloat(108.88646369499801395463383186703), bestPrice)
}

// close 10
func TestComputeBestAskBidPrice5(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(8138)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(10)
	perpetual2.AmmPositionAmount = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual2

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, false)
	Approximate(t, decimal.NewFromFloat(91.09554538669368171312465896007), bestPrice)
}

// close unsafe -10
func TestComputeBestAskBidPrice6(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(17692)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(-80)
	perpetual2.AmmPositionAmount = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual2

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, true)
	Approximate(t, decimal.NewFromFloat(100), bestPrice)
}

// close unsafe 10
func TestComputeBestAskBidPrice7(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(1996)
	perpetual0.AmmPositionAmount = decimal.NewFromFloat(80)
	perpetual2.AmmPositionAmount = decimal.NewFromFloat(10)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual0
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX1] = perpetual2

	bestPrice := ComputeBestAskBidPrice(poolStorage, TEST_PERPETUAL_INDEX0, false)
	Approximate(t, decimal.NewFromFloat(100), bestPrice)
}

// safe trader + safe amm, trader buy. open positions only
func TestComputeAMMMaxTradeAmount1(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1

	account := CopyAccountStorage(accountStorage1)
	account.WalletBalance = decimal.NewFromFloat(7005)
	amount := ComputeAMMMaxTradeAmount(poolStorage, TEST_PERPETUAL_INDEX0, account, decimal.NewFromFloat(1.1), true) // 1.1
	_, tradeIsSafe, _, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, account, amount)
	assert.Equal(t, true, tradeIsSafe)
	assert.Equal(t, true, amount.GreaterThan(decimal.NewFromFloat(1)))
	assert.Equal(t, true, amount.LessThan(decimal.NewFromFloat(1.2)))
}

// safe trader + safe amm, trader sell
func TestComputeAMMMaxTradeAmount2(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(83941.29865625)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1

	amount := ComputeAMMMaxTradeAmount(poolStorage, TEST_PERPETUAL_INDEX0, accountStorage1, decimal.NewFromFloat(-6), false) // -5.6
	account := CopyAccountStorage(accountStorage1)
	accountComputed, tradeIsSafe, _, _ := ComputeAMMTrade(poolStorage, TEST_PERPETUAL_INDEX0, account, amount)
	assert.Equal(t, true, tradeIsSafe)
	assert.Equal(t, true, amount.LessThan(decimal.NewFromFloat(-5)))
	assert.Equal(t, true, amount.GreaterThan(decimal.NewFromFloat(-6)))
	assert.Equal(t, true, accountComputed.Leverage.GreaterThan(decimal.NewFromFloat(0.99)))
	assert.Equal(t, true, accountComputed.Leverage.LessThan(decimal.NewFromFloat(1.01)))
}

// safe trader + unsafe amm(holds short), trader buy
func TestComputeAMMMaxTradeAmount3(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(17096.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(-2.3)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1

	account := CopyAccountStorage(accountStorage1)
	amount := ComputeAMMMaxTradeAmount(poolStorage, TEST_PERPETUAL_INDEX0, account, decimal.NewFromFloat(1.2), true)
	assert.Equal(t, true, amount.IsZero())
}

// safe trader + unsafe amm(holds long), trader sell
func TestComputeAMMMaxTradeAmount4(t *testing.T) {
	poolStorage := defaultPool
	poolStorage.PoolCashBalance = decimal.NewFromFloat(-13677.21634375)
	perpetual1.AmmPositionAmount = decimal.NewFromFloat(2.3)
	poolStorage.Perpetuals[TEST_PERPETUAL_INDEX0] = perpetual1
	account := CopyAccountStorage(accountStorage1)
	account.CashBalance = decimal.NewFromFloat(7698.86)
	account.PositionAmount = decimal.NewFromFloat(2.3)
	account.WalletBalance = decimal.NewFromFloat(0)
	amount := ComputeAMMMaxTradeAmount(poolStorage, TEST_PERPETUAL_INDEX0, account, decimal.NewFromFloat(0.1), false)
	assert.Equal(t, true, amount.LessThan(decimal.NewFromFloat(0.0001)))
}

func TestSlowCase20210531(t *testing.T) {
	poolStorage := &model.LiquidityPoolStorage{
		VaultFeeRate:    decimal.NewFromFloat(0.00015),
		PoolCashBalance: decimal.NewFromFloat(61439594.83745874751912811),
		Perpetuals:      make(map[int64]*model.PerpetualStorage),
	}
	perp := &model.PerpetualStorage{
		IsNormal: true,

		InitialMarginRate:      decimal.NewFromFloat(0.04),
		MaintenanceMarginRate:  decimal.NewFromFloat(0.03),
		OperatorFeeRate:        decimal.NewFromFloat(0),
		LpFeeRate:              decimal.NewFromFloat(0.00055),
		ReferrerRebateRate:     decimal.NewFromFloat(0.2),
		LiquidationPenaltyRate: decimal.NewFromFloat(0.01),
		KeeperGasReward:        decimal.NewFromFloat(30),
		InsuranceFundRate:      decimal.NewFromFloat(0.5),
		MaxOpenInterestRate:    decimal.NewFromFloat(3),

		HalfSpread:            decimal.NewFromFloat(0.0008),
		OpenSlippageFactor:    decimal.NewFromFloat(0.015),
		CloseSlippageFactor:   decimal.NewFromFloat(0.011),
		FundingRateFactor:     decimal.NewFromFloat(0.005),
		FundingRateLimit:      decimal.NewFromFloat(0.01),
		MaxLeverage:           decimal.NewFromFloat(3),
		MaxClosePriceDiscount: decimal.NewFromFloat(0.05),

		MarkPrice:               _0, // assign me later
		IndexPrice:              _0, // assign me later
		UnitAccumulativeFunding: _0, // assign me later
		OpenInterest:            _0, // assign me later
		AmmCashBalance:          _0, // assign me later
		AmmPositionAmount:       _0, // assign me later
	}
	poolStorage.Perpetuals[0] = CopyPerpetualStorage(perp)
	poolStorage.Perpetuals[0].MarkPrice = decimal.NewFromFloat(2483.26)
	poolStorage.Perpetuals[0].IndexPrice = decimal.NewFromFloat(2483.26)
	poolStorage.Perpetuals[0].UnitAccumulativeFunding = decimal.NewFromFloat(5.251555799327968135)
	poolStorage.Perpetuals[0].OpenInterest = decimal.NewFromFloat(29371.273105471586573539)
	poolStorage.Perpetuals[0].AmmCashBalance = decimal.NewFromFloat(8298681.979842171583354906)
	poolStorage.Perpetuals[0].AmmPositionAmount = decimal.NewFromFloat(-3229.051128971586573539)
	poolStorage.Perpetuals[1] = CopyPerpetualStorage(perp)
	poolStorage.Perpetuals[1].MarkPrice = decimal.NewFromFloat(36074.44)
	poolStorage.Perpetuals[1].IndexPrice = decimal.NewFromFloat(36074.44)
	poolStorage.Perpetuals[1].UnitAccumulativeFunding = decimal.NewFromFloat(19.621235681904859106)
	poolStorage.Perpetuals[1].OpenInterest = decimal.NewFromFloat(1275.166235484256783314)
	poolStorage.Perpetuals[1].AmmCashBalance = decimal.NewFromFloat(3914040.067954807914861364)
	poolStorage.Perpetuals[1].AmmPositionAmount = decimal.NewFromFloat(-104.380535484256783314)

	limitPrice := decimal.NewFromFloat(2399)
	amount := ComputeAMMAmountWithPrice(poolStorage, TEST_PERPETUAL_INDEX0, false, limitPrice)
	Approximate(t, mustNewDecimalFromString("-82085.98245157586255555"), amount)
}
