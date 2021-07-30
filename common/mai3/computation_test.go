package mai3

import (
	"testing"

	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

var defaultPool1 = &model.LiquidityPoolStorage{
	VaultFeeRate:    decimal.NewFromFloat(0.001),
	PoolCashBalance: decimal.NewFromFloat(1000), // set me later

	Perpetuals: make(map[int64]*model.PerpetualStorage), // set me later
}

var perpetual3 = &model.PerpetualStorage{
	IsNormal: true,

	MarkPrice:               decimal.NewFromFloat(1000),
	IndexPrice:              decimal.NewFromFloat(1000),
	UnitAccumulativeFunding: decimal.NewFromFloat(0),

	InitialMarginRate:      decimal.NewFromFloat(0.01),
	MaintenanceMarginRate:  decimal.NewFromFloat(0.005),
	OperatorFeeRate:        decimal.NewFromFloat(0.001),
	LpFeeRate:              decimal.NewFromFloat(0.001),
	ReferrerRebateRate:     decimal.NewFromFloat(0.2),
	LiquidationPenaltyRate: decimal.NewFromFloat(0.002),
	KeeperGasReward:        decimal.NewFromFloat(0.5),
	InsuranceFundRate:      decimal.NewFromFloat(0.5),
	OpenInterest:           decimal.NewFromFloat(0),
	MaxOpenInterestRate:    decimal.NewFromFloat(4),

	HalfSpread:            decimal.NewFromFloat(0.01),
	OpenSlippageFactor:    decimal.NewFromFloat(0.1),
	CloseSlippageFactor:   decimal.NewFromFloat(0.06),
	FundingRateLimit:      decimal.NewFromFloat(0),
	MaxLeverage:           decimal.NewFromFloat(5),
	MaxClosePriceDiscount: decimal.NewFromFloat(0.05),

	AmmCashBalance:    _0, // assign me later
	AmmPositionAmount: _0, // assign me later
}

var perpetual4 = &model.PerpetualStorage{
	IsNormal: true,

	MarkPrice:               decimal.NewFromFloat(1000),
	IndexPrice:              decimal.NewFromFloat(1000),
	UnitAccumulativeFunding: decimal.NewFromFloat(0),

	InitialMarginRate:      decimal.NewFromFloat(0.01),
	MaintenanceMarginRate:  decimal.NewFromFloat(0.005),
	OperatorFeeRate:        decimal.NewFromFloat(0.001),
	LpFeeRate:              decimal.NewFromFloat(0.001),
	ReferrerRebateRate:     decimal.NewFromFloat(0.2),
	LiquidationPenaltyRate: decimal.NewFromFloat(0.002),
	KeeperGasReward:        decimal.NewFromFloat(0.5),
	InsuranceFundRate:      decimal.NewFromFloat(0.5),
	OpenInterest:           decimal.NewFromFloat(0),
	MaxOpenInterestRate:    decimal.NewFromFloat(4),

	HalfSpread:            decimal.NewFromFloat(0.01),
	OpenSlippageFactor:    decimal.NewFromFloat(0.1),
	CloseSlippageFactor:   decimal.NewFromFloat(0.06),
	FundingRateLimit:      decimal.NewFromFloat(0),
	MaxLeverage:           decimal.NewFromFloat(5),
	MaxClosePriceDiscount: decimal.NewFromFloat(0.05),

	AmmCashBalance:    _0, // assign me later
	AmmPositionAmount: _0, // assign me later
}

var accountStorage = &model.AccountStorage{
	CashBalance:    decimal.NewFromFloat(0),
	PositionAmount: decimal.NewFromFloat(0),
	TargetLeverage: _2,
}

//addLiq + tradeWithLev long 3, short 2, short 2, long 1
func TestComputeAMMTrade1(t *testing.T) {
	poolStorage := defaultPool1
	poolStorage.Perpetuals[0] = perpetual3
	poolStorage.Perpetuals[1] = perpetual4
	poolStorage = CopyLiquidityPoolStorage(poolStorage)
	account := CopyAccountStorage(accountStorage)
	account.WalletBalance = decimal.NewFromFloat(1960.85) // for adjust margin
	// long 3 (open)
	afterTrade, tradeIsSafe, tradingPrice, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(3))
	// amm deltaCash = 3450
	// margin = cash + positionValue = 0.5 + | positionValue | / 2xLev = 1500.5. so cash = -1499.5
	// cash = deposit - 3450 - 3450 * 0.003(fee). so deposit = 1960.85
	Approximate(t, decimal.NewFromFloat(1150), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(0), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(-1499.5), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(3), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(1500.5), afterTrade.MarginBalance)
	Approximate(t, decimal.NewFromFloat(4453.45), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-3), poolStorage.Perpetuals[0].AmmPositionAmount)

	// short 2 (partial close)
	afterTrade, tradeIsSafe, tradingPrice, _ = ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(-2))
	// amm deltaCash = -2100
	// (margin - 0.5) / 1 = (1500.5 - 0.5) / 3, margin = 500.5, so cash = -499.5
	// newCash = oldCash - withdraw + 2100 - 2100 * 0.003(fee). so withdraw = 1093.7
	Approximate(t, decimal.NewFromFloat(1050), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(1093.7), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(-499.5), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(1), account.PositionAmount)
	// AMM rebalance. margin = 1000 * 1 * 1% = 10
	// amm cash + mark pos. so cash = 10 + 1000 * 1
	// final transferFee, cash += 2100 * 0.001(fee)
	Approximate(t, decimal.NewFromFloat(2355.55), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-1), poolStorage.Perpetuals[0].AmmPositionAmount)

	// short 2 (close all + open)
	afterTrade, tradeIsSafe, tradingPrice, _ = ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(-2))
	// amm deltaCash = -1984.996757074682502
	// margin = cash + positionValue = | positionValue | / 2xLev. so cash = 1500
	// idealMargin = oldCash + deltaCash + deposit - fee + mark newPos.
	// so deposit = 500.5 - (-499.5) - (1984...) + 1984... * 0.003 - (-1000) = 20.958233196541545506
	Approximate(t, decimal.NewFromFloat(992.498378537341251), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(1093.7-20.958233196541545506), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(1500.5), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(-1), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(372.53823968239218050), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(1), poolStorage.Perpetuals[0].AmmPositionAmount)

	// long 1 (close all)
	afterTrade, tradeIsSafe, tradingPrice, _ = ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(1))
	// amm deltaCash = 977.783065493367778
	Approximate(t, decimal.NewFromFloat(977.783065493367778), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(1093.7-20.958233196541545506+519.783585310152118666), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(0), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(0), account.PositionAmount)
	// AMM rebalance. margin = 1000 * 1 * 1% = 10
	// amm cash + mark pos. so cash = 10 + 1000 * 1
	// final transferFee, cash += 2100 * 0.001(fee)
	Approximate(t, decimal.NewFromFloat(1351.29908824125332628), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(0), poolStorage.Perpetuals[0].AmmPositionAmount)
}

// deposit + long 3(auto deposit on demand)
func TestComputeAMMTrade2(t *testing.T) {
	poolStorage := defaultPool1
	poolStorage.Perpetuals[0] = perpetual3
	poolStorage.Perpetuals[1] = perpetual4
	poolStorage = CopyLiquidityPoolStorage(poolStorage)
	// deposit
	account := CopyAccountStorage(accountStorage)
	account.CashBalance = decimal.NewFromFloat(500)
	account.WalletBalance = decimal.NewFromFloat(1460.85)

	// long 3 (open)
	afterTrade, tradeIsSafe, tradingPrice, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(3))
	// amm deltaCash = 3450
	// margin = cash + positionValue = 0.5 + | positionValue | / 2xLev = 1500.5. so cash = -1499.5
	// newCash = oldCash + deposit - 3450 - 3450 * 0.003(fee). so deposit = 1460.85
	Approximate(t, decimal.NewFromFloat(1150), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(0), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(-1499.5), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(3), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(1500.5), afterTrade.MarginBalance)

	Approximate(t, decimal.NewFromFloat(4453.45), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-3), poolStorage.Perpetuals[0].AmmPositionAmount)
}

// short 1 when MM < margin < IM, normal fees
func TestComputeAMMTrade3(t *testing.T) {
	poolStorage := defaultPool1
	poolStorage.Perpetuals[0] = perpetual3
	poolStorage.Perpetuals[1] = perpetual4
	poolStorage = CopyLiquidityPoolStorage(poolStorage)
	// deposit
	account := CopyAccountStorage(accountStorage)
	account.WalletBalance = decimal.NewFromFloat(1960.85)
	// long 3 (open)
	afterTrade, _, _, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(3))
	// close when MM < margin < IM, normal fees
	poolStorage.Perpetuals[0].MarkPrice = decimal.NewFromFloat(505)
	poolStorage.Perpetuals[0].IndexPrice = decimal.NewFromFloat(505)
	assert.Equal(t, true, afterTrade.IsIMSafe)
	afterTrade, tradeIsSafe, tradingPrice, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(-1))
	// amm deltaCash = -515.541132467602916841
	// newMargin = newCash + 505 * 2 = 505 * 2 * 0.01 + 0.5. so cash = -999.4
	// newCash = oldCash - withdraw + 515... - 515... * 0.003(fee). so withdraw = 13.894509070200108090477
	Approximate(t, decimal.NewFromFloat(515.541132467602916841), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(13.894509070200108090477), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(-999.4), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(2), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(3938.42440866486468608), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-2), poolStorage.Perpetuals[0].AmmPositionAmount)
}

// short 1 when margin < mm, the profit is large enough, normal fees
func TestComputeAMMTrade4(t *testing.T) {
	poolStorage := defaultPool1
	poolStorage.Perpetuals[0] = perpetual3
	poolStorage.Perpetuals[1] = perpetual4
	poolStorage = CopyLiquidityPoolStorage(poolStorage)
	// deposit
	account := CopyAccountStorage(accountStorage)
	account.WalletBalance = decimal.NewFromFloat(1960.85)
	// long 3 (open)
	ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(3))

	poolStorage.Perpetuals[0].MarkPrice = decimal.NewFromFloat(501)
	poolStorage.Perpetuals[0].IndexPrice = decimal.NewFromFloat(501)
	account1 := CopyAccountStorage(accountStorage)
	account1.WalletBalance = decimal.NewFromFloat(3560.35)

	ComputeAMMTrade(poolStorage, 0, account1, decimal.NewFromFloat(2))
	// amm deltaCash = 1070.964429859700685024
	Approximate(t, decimal.NewFromFloat(5525.485394289560385709), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-5), poolStorage.Perpetuals[0].AmmPositionAmount)

	// close when margin < MM, but profit is large, normal fees
	afterTrade, tradeIsSafe, tradingPrice, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(-1))
	// amm deltaCash = -521.201994206724030199
	// old lev = 501x, margin = 501 * 2 * 1% = cash + 501 * 2 + 0.5
	// cash = oldCash + deltaCash - fee - withdraw. so withdraw = 11.618388224103858108403
	Approximate(t, decimal.NewFromFloat(521.201994206724030199), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(11.618388224103858108403), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(-991.48), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(2), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(5004.80460207704307954), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-4), poolStorage.Perpetuals[0].AmmPositionAmount)
}

// a very small amount. long small amount from 0 position, margin = value / lev + keeperGasReward
func TestComputeAMMTrade5(t *testing.T) {
	poolStorage := defaultPool1
	poolStorage.Perpetuals[0] = perpetual3
	poolStorage.Perpetuals[1] = perpetual4
	poolStorage = CopyLiquidityPoolStorage(poolStorage)
	// deposit
	account := CopyAccountStorage(accountStorage)
	account.WalletBalance = decimal.NewFromFloat(0.500051303)
	// long 1e-7 (open)
	afterTrade, tradeIsSafe, tradingPrice, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(1e-7))
	// amm deltaCash = 0.000101
	// margin = cash + positionValue = 0.5 + | positionValue | / 2xLev = 0.50005. so cash = 0.49995
	// deposit = newCash + deltaCash = 0.49995 + 1010 * 1e-7 + 1010 * 1e-7 * 0.003 = 0.500051303
	Approximate(t, decimal.NewFromFloat(1010), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(0), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(0.49995), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(1e-7), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(0.50005), afterTrade.MarginBalance)

	Approximate(t, decimal.NewFromFloat(1000.000101101), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-1e-7), poolStorage.Perpetuals[0].AmmPositionAmount)
}

// long + long
func TestComputeAMMTrade6(t *testing.T) {
	poolStorage := defaultPool1
	poolStorage.Perpetuals[0] = perpetual3
	poolStorage.Perpetuals[1] = perpetual4
	poolStorage = CopyLiquidityPoolStorage(poolStorage)
	// deposit
	account := CopyAccountStorage(accountStorage)
	account.CashBalance = decimal.NewFromFloat(500)
	account.WalletBalance = decimal.NewFromFloat(1460.85)

	// long 3 (open)

	ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(3))
	Approximate(t, decimal.NewFromFloat(0), account.WalletBalance)

	// long 1 (open)

	account.WalletBalance = decimal.NewFromFloat(851.872666114466336438)
	afterTrade, tradeIsSafe, tradingPrice, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(1))

	// amm deltaCash = 1347.829178578730146
	// deposit = deltaPosition * mark / 2xLev + pnl + fee = 1000 / 2 + 347.829178578730146 + 1347.829178578730146 * 0.003 = 851.872666114466336438
	// newCash = old cash - deltaCash + deposit - fee = -1499.5 - 1347.829178578730146 + 847.829178578730146 = -1999.5
	// margin = newCash + mark * position = -1999.5 + 4 * 1000 = 2000.5
	Approximate(t, decimal.NewFromFloat(1347.829178578730146), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(0), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(-1999.5), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(4), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(2000.5), afterTrade.MarginBalance)

	Approximate(t, decimal.NewFromFloat(5802.627007757308876146), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-4), poolStorage.Perpetuals[0].AmmPositionAmount)
}

// unsafe long + long
func TestComputeAMMTrade7(t *testing.T) {
	poolStorage := defaultPool1
	poolStorage.Perpetuals[0] = perpetual3
	poolStorage.Perpetuals[1] = perpetual4
	poolStorage = CopyLiquidityPoolStorage(poolStorage)
	// deposit
	account := CopyAccountStorage(accountStorage)
	account.CashBalance = decimal.NewFromFloat(500)
	account.WalletBalance = decimal.NewFromFloat(1460.85)

	// long 3 (open)
	ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(3))
	Approximate(t, decimal.NewFromFloat(0), account.WalletBalance)

	poolStorage.Perpetuals[0].MarkPrice = decimal.NewFromFloat(100)
	poolStorage.Perpetuals[0].IndexPrice = decimal.NewFromFloat(100)
	afterTrade, _ := ComputeAccount(poolStorage, 0, account)
	assert.Equal(t, false, afterTrade.IsIMSafe)

	// long 1 (open)
	account.WalletBalance = decimal.NewFromFloat(1206.034893526402413359)
	afterTrade, tradeIsSafe, tradingPrice, _ := ComputeAMMTrade(poolStorage, 0, account, decimal.NewFromFloat(1))

	// amm deltaCash = 101.729704413161927575
	// margin = initialMargin = 4 * 100 * 0.01 = 4
	// newCash = margin - mark * pos = 4 - 100 * 4 = -396
	// deposit = newMargin - oldMargin - pnl + fee = 4.5 - (-1199.5) - (-1.729704413161927575) + 101.729704413161927575 * 0.003 = 1206.034893526401413357725
	Approximate(t, decimal.NewFromFloat(101.729704413161927575), tradingPrice)
	assert.Equal(t, true, afterTrade.IsMMSafe)
	assert.Equal(t, true, tradeIsSafe)
	Approximate(t, decimal.NewFromFloat(0), account.WalletBalance)
	Approximate(t, decimal.NewFromFloat(-395.5), account.CashBalance)
	Approximate(t, decimal.NewFromFloat(4), account.PositionAmount)
	Approximate(t, decimal.NewFromFloat(4.5), afterTrade.MarginBalance)

	Approximate(t, decimal.NewFromFloat(4555.281434117575089503), poolStorage.PoolCashBalance)
	Approximate(t, decimal.NewFromFloat(-4), poolStorage.Perpetuals[0].AmmPositionAmount)
}
