package mai3

import (
	"fmt"
	"github.com/mcarloai/mai-v3-broker/common/model"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

func ComputeAMMAmountWithPrice(p *model.LiquidityPoolStorage, perpetualIndex int64, isTraderBuy bool, limitPrice decimal.Decimal) decimal.Decimal {
	perpetual, ok := p.Perpetuals[perpetualIndex]
	if !ok {
		logger.Warnf("perpetual %d not found in the pool", perpetualIndex)
		return _0
	}

	if isTraderBuy {
		limitPrice = limitPrice.Div(_1.Add(perpetual.HalfSpread))
	} else {
		limitPrice = limitPrice.Div(_1.Sub(perpetual.HalfSpread))
	}

	isAMMBuy := !isTraderBuy
	context := initAMMTradingContext(p, perpetualIndex)
	if context.Position1.LessThanOrEqual(_0) && !isAMMBuy {
		return computeAMMOpenAmountWithPrice(context, limitPrice, isAMMBuy).Neg()
	} else if context.Position1.LessThan(_0) && isAMMBuy {
		return computeAMMCloseAndOpenAmountWithPrice(context, limitPrice, isAMMBuy).Neg()
	} else if context.Position1.GreaterThanOrEqual(_0) && isAMMBuy {
		return computeAMMOpenAmountWithPrice(context, limitPrice, isAMMBuy).Neg()
	} else if context.Position1.GreaterThan(_0) && !isAMMBuy {
		return computeAMMCloseAndOpenAmountWithPrice(context, limitPrice, isAMMBuy).Neg()
	}

	logger.Errorf("bug: unknown trading direction")
	return _0
}

func copyAMMTradingContext(ammContext *model.AMMTradingContext) *model.AMMTradingContext {
	return &model.AMMTradingContext{
		Index:                        ammContext.Index,
		Position1:                    ammContext.Position1,
		HalfSpread:                   ammContext.HalfSpread,
		OpenSlippageFactor:           ammContext.OpenSlippageFactor,
		CloseSlippageFactor:          ammContext.CloseSlippageFactor,
		FundingRateLimit:             ammContext.FundingRateLimit,
		MaxLeverage:                  ammContext.MaxLeverage,
		OtherIndex:                   ammContext.OtherIndex,
		OtherPosition:                ammContext.OtherPosition,
		OtherHalfSpread:              ammContext.OtherHalfSpread,
		OtherOpenSlippageFactor:      ammContext.OtherOpenSlippageFactor,
		OtherCloseSlippageFactor:     ammContext.OtherCloseSlippageFactor,
		OtherFundingRateCoefficient:  ammContext.OtherFundingRateCoefficient,
		OtherMaxLeverage:             ammContext.OtherMaxLeverage,
		Cash:                         ammContext.Cash,
		PoolMargin:                   ammContext.PoolMargin,
		DeltaMargin:                  ammContext.DeltaMargin,
		DeltaPosition:                ammContext.DeltaPosition,
		ValueWithoutCurrent:          ammContext.ValueWithoutCurrent,
		SquareValueWithoutCurrent:    ammContext.SquareValueWithoutCurrent,
		PositionMarginWithoutCurrent: ammContext.PositionMarginWithoutCurrent,
	}
}

func computeAMMOpenAmountWithPrice(context *model.AMMTradingContext, limitPrice decimal.Decimal, isAMMBuy bool) decimal.Decimal {
	if isAMMBuy && context.Position1.LessThan(_0) ||
		!isAMMBuy && context.Position1.GreaterThan(_0) {
		logger.Errorf("this is not opening. pos1: %s isBuy: %v", context.Position1, isAMMBuy)
		return _0
	}
	if !isAMMSafe(context, context.OpenSlippageFactor) {
		return _0
	}

	if err := computeAMMPoolMargin(context, context.OpenSlippageFactor); err != nil {
		logger.Errorf("computeAMMOpenAmountWithPrice: computeAMMPoolMargin fail:%s", err)
		return _0
	}

	safePos2 := _0
	if isAMMBuy {
		safePos2 = computeAMMSafeLongPositionAmount(context, context.OpenSlippageFactor)
		if safePos2.LessThan(context.Position1) {
			return _0
		}
	} else {
		safePos2 = computeAMMSafeShortPositionAmount(context, context.OpenSlippageFactor)
		if safePos2.GreaterThan(_0) {
			return _0
		}
	}

	maxAmount := safePos2.Sub(context.Position1)
	safePos2Context, err := computeAMMInternalOpen(context, maxAmount)
	if err != nil {
		logger.Errorf("computeAMMInternalOpen fail:%s", err)
		return _0
	}
	if !maxAmount.Equal(safePos2Context.DeltaPosition.Sub(context.DeltaPosition)) {
		logger.Errorf("open positions failed")
		return _0
	}

	safePos2Price := safePos2Context.DeltaMargin.Div(safePos2Context.DeltaPosition).Abs()
	if (isAMMBuy && safePos2Price.GreaterThanOrEqual(limitPrice)) ||
		(!isAMMBuy && safePos2Price.LessThanOrEqual(limitPrice)) {
		return maxAmount
	}

	amount, err := computeAMMInverseVWAP(context, limitPrice, context.OpenSlippageFactor, isAMMBuy)
	if err != nil {
		logger.Errorf("computeAMMOpenAmountWithPrice: computeAMMInverseVWAP failed:%s", err)
		return _0
	}
	if (isAMMBuy && amount.GreaterThan(_0)) ||
		(!isAMMBuy && amount.LessThan(_0)) {
		return amount
	}
	return _0
}

func computeAMMCloseAndOpenAmountWithPrice(context *model.AMMTradingContext, limitPrice decimal.Decimal, isAMMBuy bool) decimal.Decimal {
	if !context.DeltaMargin.IsZero() || !context.DeltaPosition.IsZero() {
		logger.Errorf("computeAMMCloseAndOpenAmountWithPrice: partial close is not supported")
		return _0
	}
	if context.Position1.IsZero() {
		logger.Errorf("computeAMMCloseAndOpenAmountWithPrice: close from 0 is not supported")
		return _0
	}

	var err error

	zeroContext, err := computeAMMInternalClose(context, context.Position1.Neg())
	if err != nil {
		logger.Errorf("computeAMMCloseAndOpenAmountWithPrice: computeAMMInternalClose err:%s", err)
		return _0
	}
	if zeroContext.DeltaPosition.IsZero() {
		logger.Errorf("close to zero failed")
		return _0
	}
	zeroPrice := zeroContext.DeltaMargin.Div(zeroContext.DeltaPosition).Abs()
	if isAMMBuy && zeroPrice.GreaterThanOrEqual(limitPrice) ||
		!isAMMBuy && zeroPrice.LessThanOrEqual(limitPrice) {
		// close all
		context = zeroContext
	} else if !isAMMSafe(context, context.CloseSlippageFactor) {
		// case 2: unsafe close, but price not matched
		return _0
	} else {
		// case 3: close by price
		err := computeAMMPoolMargin(context, context.CloseSlippageFactor)
		if err != nil {
			logger.Errorf("computeAMMCloseAndOpenAmountWithPrice: computeAMMPoolMargin err:%s", err)
			return _0
		}
		amount, err := computeAMMInverseVWAP(context, limitPrice, context.CloseSlippageFactor, isAMMBuy)
		if err != nil {
			logger.Errorf("computeAMMCloseAndOpenAmountWithPrice: computeAMMInverseVWAP failed:%s", err)
			return _0
		}
		if (isAMMBuy && amount.GreaterThan(_0)) ||
			(!isAMMBuy && amount.LessThan(_0)) {
			if context, err = computeAMMInternalClose(context, amount); err != nil {
				logger.Errorf("computeAMMCloseAndOpenAmountWithPrice: computeAMMInternalClose failed:%s", err)
				return _0
			}
		}
	}
	if (isAMMBuy && context.Position1.GreaterThanOrEqual(_0)) ||
		(!isAMMBuy && context.Position1.LessThanOrEqual(_0)) {
		openAmount := computeAMMOpenAmountWithPrice(context, limitPrice, isAMMBuy)
		return context.DeltaPosition.Add(openAmount)
	}
	return context.DeltaPosition
}

func computeAMMInternalClose(context *model.AMMTradingContext, amount decimal.Decimal) (*model.AMMTradingContext, error) {
	beta := context.CloseSlippageFactor
	ret := copyAMMTradingContext(context)
	index := ret.Index
	position2 := ret.Position1.Add(amount)
	deltaMargin := _0
	var err error
	if isAMMSafe(ret, beta) {
		err = computeAMMPoolMargin(ret, beta)
		if err != nil {
			return nil, err
		}
		deltaMargin, err = computeDeltaMargin(ret, beta, position2)
		if err != nil {
			return nil, err
		}
	} else {
		deltaMargin = index.Mul(amount).Neg()
	}

	ret.DeltaMargin = ret.DeltaMargin.Add(deltaMargin)
	ret.DeltaPosition = ret.DeltaPosition.Add(amount)
	ret.Cash = ret.Cash.Add(deltaMargin)
	ret.Position1 = position2
	return ret, nil
}

func computeAMMInternalOpen(context *model.AMMTradingContext, amount decimal.Decimal) (*model.AMMTradingContext, error) {
	beta := context.OpenSlippageFactor
	ret := copyAMMTradingContext(context)
	position2 := ret.Position1.Add(amount)
	deltaMargin := _0
	if !isAMMSafe(ret, beta) {
		return nil, fmt.Errorf("unsafe amm")
	}

	if err := computeAMMPoolMargin(ret, beta); err != nil {
		return nil, err
	}
	if amount.GreaterThan(_0) {
		// 0.....position2.....safePosition2
		safePosition2 := computeAMMSafeLongPositionAmount(ret, beta)
		if position2.GreaterThan(safePosition2) {
			return nil, fmt.Errorf("AMM can not open position anymore: position too large after trade")
		}
	} else {
		// safePosition2.....position2.....0
		safePosition2 := computeAMMSafeShortPositionAmount(ret, beta)
		if position2.LessThan(safePosition2) {
			return nil, fmt.Errorf("AMM can not open position anymore: position too large after trade")
		}
	}

	deltaMargin, err := computeDeltaMargin(ret, beta, position2)
	if err != nil {
		return nil, err
	}
	ret.DeltaMargin = ret.DeltaMargin.Add(deltaMargin)
	ret.DeltaPosition = ret.DeltaPosition.Add(amount)
	ret.Cash = ret.Cash.Add(deltaMargin)
	ret.Position1 = position2
	return ret, nil
}

// the inverse function of VWAP of AMM pricing function
// call computeAMMPoolMargin before this function
// the returned amount(= pos2 - pos1) is the AMM's perspective
// make sure ammSafe before this function
func computeAMMInverseVWAP(context *model.AMMTradingContext, price, beta decimal.Decimal, isAMMBuy bool) (decimal.Decimal, error) {
	previousMa1MinusMa2 := context.DeltaMargin.Neg()
	previousAmount := context.DeltaPosition
	/*
	  A = P_i β;
	  B = -2 P_i M + 2 A N1 + 2 M price;
	  C = -2 M (previousMa1MinusMa2 - previousAmount price);
	  sols = (-B ± sqrt(B^2 - 4 A C)) / (2 A);
	*/
	a := context.Index.Mul(beta)
	denominator := a.Mul(_2)
	if denominator.IsZero() {
		return _0, fmt.Errorf("computeAMMInverseVWAP: bad perpetual parameter beta or index")
	}
	b := context.Index.Mul(context.PoolMargin).Neg()
	b = b.Add(a.Mul(context.Position1))
	b = b.Add(context.PoolMargin.Mul(price))
	b = b.Mul(_2)
	c := previousMa1MinusMa2.Sub(previousAmount.Mul(price)).Mul(context.PoolMargin).Mul(_2).Neg()
	beforeSqrt := a.Mul(c).Mul(_4).Neg().Add(b.Mul(b))
	if beforeSqrt.LessThan(_0) {
		return _0, fmt.Errorf("computeAMMInverseVWAP: impossible price. beforeSqrt")
	}
	numerator := Sqrt(beforeSqrt)
	if !isAMMBuy {
		numerator = numerator.Neg()
	}
	numerator = numerator.Sub(b)
	return numerator.Div(denominator), nil
}

func computeDeltaMargin(context *model.AMMTradingContext, beta, position2 decimal.Decimal) (decimal.Decimal, error) {
	if context.Position1.GreaterThan(_0) && position2.LessThan(_0) ||
		context.Position1.LessThan(_0) && position2.GreaterThan(_0) {
		return _0, fmt.Errorf("computeDeltaMargin: cross direction is not supported")
	}
	if context.PoolMargin.LessThanOrEqual(_0) {
		return _0, fmt.Errorf("computeDeltaMargin: AMM poolMargin <= 0")
	}
	// P_i (N1 - N2) (1 - β / M * (N2 + N1) / 2)
	ret := position2.Add(context.Position1).Div(_2).Div(context.PoolMargin).Mul(beta)
	ret = _1.Sub(ret)
	ret = context.Position1.Sub(position2).Mul(ret).Mul(context.Index)
	return ret, nil
}

func isAMMSafe(context *model.AMMTradingContext, beta decimal.Decimal) bool {
	valueWithCurrent := context.ValueWithoutCurrent.Add(context.Index.Mul(context.Position1))
	squareValueWithCurrent := context.SquareValueWithoutCurrent.
		Add(beta.Mul(context.Index).Mul(context.Position1).Mul(context.Position1))
	// √(2 Σ(β_j P_i_j N_j)) - Σ(P_i_j N_j). always positive
	beforeSqrt := _2.Mul(squareValueWithCurrent)
	safeCash := Sqrt(beforeSqrt).Sub(valueWithCurrent)
	return context.Cash.GreaterThanOrEqual(safeCash)
}

func computeAMMPoolMargin(context *model.AMMTradingContext, beta decimal.Decimal) error {
	marginBalanceWithCurrent := context.Cash.
		Add(context.ValueWithoutCurrent).
		Add(context.Index.Mul(context.Position1))
	squareValueWithCurrent := context.SquareValueWithoutCurrent.
		Add(beta.Mul(context.Index).Mul(context.Position1).Mul(context.Position1))
	beforeSqrt := marginBalanceWithCurrent.Mul(marginBalanceWithCurrent).Sub(_2.Mul(squareValueWithCurrent))
	if beforeSqrt.LessThan(_0) {
		return fmt.Errorf("AMM available margin sqrt < 0")
	}
	poolMargin := marginBalanceWithCurrent.Add(Sqrt(beforeSqrt)).Div(_2)
	context.PoolMargin = poolMargin
	return nil
}

func computeAMMSafeShortPositionAmount(context *model.AMMTradingContext, beta decimal.Decimal) decimal.Decimal {
	condition3, ok := computeAMMSafeCondition3(context, beta)
	if !ok {
		return _0
	}
	condition3 = condition3.Neg()
	condition2, ok := computeAMMSafeCondition2(context, beta)
	if ok {
		return condition3
	}
	condition2 = condition2.Neg()
	return decimal.Max(condition2, condition3)
}

func computeAMMSafeLongPositionAmount(context *model.AMMTradingContext, beta decimal.Decimal) decimal.Decimal {
	condition3, ok := computeAMMSafeCondition3(context, beta)
	if !ok {
		return _0
	}
	condition1 := computeAMMSafeCondition1(context, beta)
	condition13 := decimal.Min(condition1, condition3)
	condition2, ok := computeAMMSafeCondition2(context, beta)
	if ok {
		return condition13
	}
	return decimal.Min(condition2, condition13)
}

func computeAMMSafeCondition1(context *model.AMMTradingContext, beta decimal.Decimal) decimal.Decimal {
	// M / β
	return context.PoolMargin.Div(beta)
}

// return true if always safe
func computeAMMSafeCondition2(context *model.AMMTradingContext, beta decimal.Decimal) (decimal.Decimal, bool) {
	// M - Σ(positionMargin_j - squareValue_j / 2 / M) where j ≠ id
	x := context.PoolMargin.Sub(context.PositionMarginWithoutCurrent).
		Add(context.SquareValueWithoutCurrent.Div(context.PoolMargin).Div(_2))
		//  M - √(M(M - 2βλ^2/P_i x))
	// ---------------------------
	//             βλ
	beforeSqrt := x.Mul(context.MaxLeverage).Mul(context.MaxLeverage).Mul(beta).Mul(_2).Div(context.Index)
	beforeSqrt = context.PoolMargin.Sub(beforeSqrt).Mul(context.PoolMargin)
	if beforeSqrt.LessThan(_0) {
		return _0, true
	}
	position2 := context.PoolMargin.Sub(Sqrt(beforeSqrt))
	position2 = position2.Div(beta).Div(context.MaxLeverage)
	return position2, false
}

// return false if always safe
func computeAMMSafeCondition3(context *model.AMMTradingContext, beta decimal.Decimal) (decimal.Decimal, bool) {
	//    2M^2 - squareValueWithoutCurrent
	// √(----------------------------------)
	//                P_i β
	beforeSqrt := _2.Mul(context.PoolMargin).Mul(context.PoolMargin).
		Sub(context.SquareValueWithoutCurrent).
		Div(context.Index).Div(beta)
	if beforeSqrt.LessThan(_0) {
		return _0, false
	}
	return Sqrt(beforeSqrt), true
}

func initAMMTradingContext(p *model.LiquidityPoolStorage, perpetualIndex int64) *model.AMMTradingContext {
	if _, ok := p.Perpetuals[perpetualIndex]; !ok {
		return nil
	}
	index := _0
	position1 := _0
	halfSpread := _0
	openSlippageFactor := _0
	closeSlippageFactor := _0
	fundingRateLimit := _0
	maxLeverage := _0

	otherIndex := make([]decimal.Decimal, 0)
	otherPosition := make([]decimal.Decimal, 0)
	otherHalfSpread := make([]decimal.Decimal, 0)
	otherOpenSlippageFactor := make([]decimal.Decimal, 0)
	otherCloseSlippageFactor := make([]decimal.Decimal, 0)
	otherFundingRateCoefficient := make([]decimal.Decimal, 0)
	otherMaxLeverage := make([]decimal.Decimal, 0)

	// split perpetuals into current perpetual and other perpetuals
	// M_c = ammCash - Σ accumulatedFunding * N
	cash := p.PoolCashBalance
	for id, perpetual := range p.Perpetuals {
		cash = cash.Sub(perpetual.UnitAccumulativeFunding.Mul(perpetual.AmmPositionAmount))
		if id == perpetualIndex {
			index = perpetual.IndexPrice
			position1 = perpetual.AmmPositionAmount
			halfSpread = perpetual.HalfSpread
			openSlippageFactor = perpetual.OpenSlippageFactor
			closeSlippageFactor = perpetual.CloseSlippageFactor
			fundingRateLimit = perpetual.FundingRateLimit
			maxLeverage = perpetual.MaxLeverage
		} else {
			otherIndex = append(otherIndex, perpetual.IndexPrice)
			otherPosition = append(otherPosition, perpetual.AmmPositionAmount)
			otherHalfSpread = append(otherHalfSpread, perpetual.HalfSpread)
			otherOpenSlippageFactor = append(otherOpenSlippageFactor, perpetual.OpenSlippageFactor)
			otherCloseSlippageFactor = append(otherCloseSlippageFactor, perpetual.CloseSlippageFactor)
			otherFundingRateCoefficient = append(otherFundingRateCoefficient, perpetual.FundingRateLimit)
			otherMaxLeverage = append(otherMaxLeverage, perpetual.MaxLeverage)
		}
	}
	ret := &model.AMMTradingContext{
		Index:                        index,
		Position1:                    position1,
		HalfSpread:                   halfSpread,
		OpenSlippageFactor:           openSlippageFactor,
		CloseSlippageFactor:          closeSlippageFactor,
		FundingRateLimit:             fundingRateLimit,
		MaxLeverage:                  maxLeverage,
		OtherIndex:                   otherIndex,
		OtherPosition:                otherPosition,
		OtherHalfSpread:              otherHalfSpread,
		OtherOpenSlippageFactor:      otherOpenSlippageFactor,
		OtherCloseSlippageFactor:     otherCloseSlippageFactor,
		OtherFundingRateCoefficient:  otherFundingRateCoefficient,
		OtherMaxLeverage:             otherMaxLeverage,
		Cash:                         cash,
		PoolMargin:                   _0,
		DeltaMargin:                  _0,
		DeltaPosition:                _0,
		ValueWithoutCurrent:          _0,
		SquareValueWithoutCurrent:    _0,
		PositionMarginWithoutCurrent: _0,
	}
	initAMMTradingContextEagerEvaluation(ret)
	return ret
}

func initAMMTradingContextEagerEvaluation(context *model.AMMTradingContext) {
	valueWithoutCurrent := _0
	squareValueWithoutCurrent := _0
	positionMarginWithoutCurrent := _0
	for j := 0; j < len(context.OtherIndex); j++ {
		// Σ_j (P_i N) where j ≠ id
		valueWithoutCurrent = valueWithoutCurrent.Add(
			context.OtherIndex[j].Mul(context.OtherPosition[j]))
		// Σ_j (β P_i N^2) where j ≠ id
		squareValueWithoutCurrent = squareValueWithoutCurrent.Add(
			context.OtherOpenSlippageFactor[j].Mul(context.OtherIndex[j]).Mul(context.OtherPosition[j]).Mul(context.OtherPosition[j]))
		// Σ_j (P_i_j * | N_j | / λ_j) where j ≠ id
		positionMarginWithoutCurrent = positionMarginWithoutCurrent.Add(
			context.OtherIndex[j].Mul(context.OtherPosition[j].Abs()).Div(context.OtherMaxLeverage[j]))
	}

	context.ValueWithoutCurrent = valueWithoutCurrent
	context.SquareValueWithoutCurrent = squareValueWithoutCurrent
	context.PositionMarginWithoutCurrent = positionMarginWithoutCurrent
}