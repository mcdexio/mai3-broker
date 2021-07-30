package match

import (
	"sort"
	"time"

	"github.com/mcdexio/mai3-broker/common/mai3"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/common/orderbook"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

var TradeAmountRelaxFactor = decimal.NewFromFloat(0.99)
var _0 = decimal.Zero
var _1 = decimal.NewFromInt(1)

func splitActiveOrders(orders []*model.Order) (buys, sells []*model.Order) {
	for _, order := range orders {
		order.CheckAmount = order.AvailableAmount.Add(order.PendingAmount)
		if order.CheckAmount.LessThan(decimal.Zero) {
			// sell
			sells = append(sells, order)
		} else if order.CheckAmount.GreaterThan(decimal.Zero) {
			// buy
			buys = append(buys, order)
		}
	}
	sort.Slice(buys, func(i, j int) bool {
		return buys[i].Price.GreaterThan(buys[j].Price)
	})
	sort.Slice(sells, func(i, j int) bool {
		return sells[i].Price.LessThan(sells[j].Price)
	})
	return
}

func openOrderCost(pool *model.LiquidityPoolStorage, perpetualIndex int64, order *model.Order, leverage decimal.Decimal) (cost, fee, potentialLoss decimal.Decimal) {
	perp, ok := pool.Perpetuals[perpetualIndex]
	if !ok {
		return
	}
	amount := order.CheckAmount
	feeRate := pool.VaultFeeRate.Add(perp.LpFeeRate).Add(perp.OperatorFeeRate)
	potentialPNL := perp.MarkPrice.Sub(order.Price).Mul(amount)
	// loss = pnl if pnl < 0 else 0
	potentialLoss = decimal.Min(_0, potentialPNL)
	// fee = limitPrice * | amount | * feeRate
	fee = order.Price.Mul(amount.Abs()).Mul(feeRate)
	margin := _0
	if amount.LessThan(_0) && order.Price.LessThan(perp.MarkPrice) {
		// mark * | amount | / lev
		margin = perp.MarkPrice.Mul(amount.Abs()).Div(leverage)
	} else {
		// limitPrice * | amount | / lev
		margin = order.Price.Mul(amount.Abs()).Div(leverage)
	}
	cost = margin.Add(fee).Sub(potentialLoss)
	return
}

func sideAvailable(pool *model.LiquidityPoolStorage, perpetualIndex int64, marginBalance, position, targetLeverage, walletBalance decimal.Decimal, orders []*model.Order) (cancels []*OrderCancel, remainWalletBalance decimal.Decimal) {
	cancels = make([]*OrderCancel, 0)
	remainWalletBalance = walletBalance
	if len(orders) == 0 {
		return
	}
	perpetual, ok := pool.Perpetuals[perpetualIndex]
	if !ok {
		return
	}

	feeRate := pool.VaultFeeRate.Add(perpetual.LpFeeRate).Add(perpetual.OperatorFeeRate)
	remainPosition := position
	remainMargin := marginBalance
	remainOrders := make([]*model.Order, 0)
	for _, order := range orders {
		amount := order.CheckAmount
		close, _ := utils.SplitAmount(remainPosition, amount)
		if !close.IsZero() {
			newPosition := remainPosition.Add(close)
			newPositionMargin := perpetual.MarkPrice.Mul(newPosition.Abs()).Mul(perpetual.InitialMarginRate)
			if !newPosition.IsZero() {
				newPositionMargin = newPositionMargin.Add(perpetual.KeeperGasReward)
			}
			potentialPNL := perpetual.MarkPrice.Sub(order.Price).Mul(close)
			// loss = pnl if pnl < 0 else 0
			potentialLoss := decimal.Min(potentialPNL, _0)
			afterMargin := remainMargin.Add(potentialLoss)
			fee := _0
			if close.Equal(amount) {
				// close only
				fee = decimal.Min(
					// marginBalance + pnl - mark * | newPosition | * imRate
					decimal.Max(afterMargin.Sub(newPositionMargin), _0),
					order.Price.Mul(close.Abs()).Mul(feeRate),
				)
			} else {
				// close + open
				fee = order.Price.Mul(close.Abs()).Mul(feeRate)
			}

			afterMargin = afterMargin.Sub(fee)
			if afterMargin.LessThan(_0) {
				// bankrupt when close. pretend all orders as open orders
				remainPosition = _0
				remainMargin = _0
				remainOrders = append(remainOrders, order)
			} else {
				// !bankrupt
				withdraw := _0
				if afterMargin.GreaterThanOrEqual(newPositionMargin) {
					// withdraw only if marginBalance >= IM
					// withdraw = afterMargin - reserved2 - (remainMargin - reserved1) * (1 - | close / remainPosition |)
					withdraw = close.Div(remainPosition).Abs()
					withdraw = _1.Sub(withdraw).Mul(remainMargin.Sub(perpetual.KeeperGasReward))
					withdraw = afterMargin.Sub(withdraw)
					if !newPosition.IsZero() {
						withdraw = withdraw.Sub(perpetual.KeeperGasReward)
					}
					// never deposit when close
					withdraw = decimal.Max(_0, withdraw)
				}
				remainMargin = afterMargin.Sub(withdraw)
				remainWalletBalance = remainWalletBalance.Add(withdraw)
				remainPosition = remainPosition.Add(close)
				newOrderAmount := amount.Sub(close)
				if !newOrderAmount.IsZero() {
					order.CheckAmount = newOrderAmount
					remainOrders = append(remainOrders, order)
				}
			}
		} else {
			remainOrders = append(remainOrders, order)
		}
	}

	// if close = 0 && position = 0 && margin > 0
	if remainPosition.IsZero() {
		remainWalletBalance = remainWalletBalance.Add(remainMargin)
		remainMargin = _0
	}

	// open position
	for _, order := range remainOrders {
		cost, fee, potentialLoss := openOrderCost(pool, perpetualIndex, order, targetLeverage)
		if remainPosition.IsZero() {
			cost = cost.Add(perpetual.KeeperGasReward)
		}
		remainPosition = remainPosition.Add(order.CheckAmount)
		remainMargin = remainMargin.Add(potentialLoss).Sub(fee)
		// at least IM and keeperGasReward
		im := perpetual.MarkPrice.Mul(remainPosition.Abs()).Mul(perpetual.InitialMarginRate).Add(perpetual.KeeperGasReward)
		cost = decimal.Max(
			im.Sub(remainMargin),
			cost,
		)
		remainMargin = remainMargin.Add(cost)
		remainWalletBalance = remainWalletBalance.Sub(cost)
		if remainWalletBalance.LessThan(_0) {
			cancel := &OrderCancel{
				LiquidityPoolAddress: order.LiquidityPoolAddress,
				PerpetualIndex:       order.PerpetualIndex,
				OrderHash:            order.OrderHash,
				Status:               order.Status,
				ToCancel:             order.AvailableAmount,
				Reason:               model.CancelReasonInsufficientFunds,
			}
			cancels = append(cancels, cancel)
		}
	}

	return
}

func ComputeOrderAvailable(poolStorage *model.LiquidityPoolStorage, perpetualIndex int64, account *model.AccountStorage, orders []*model.Order) ([]*OrderCancel, decimal.Decimal) {
	cancels := make([]*OrderCancel, 0)
	buyOrders, sellOrders := splitActiveOrders(orders)
	computedAccount, err := mai3.ComputeAccount(poolStorage, perpetualIndex, account)
	if err != nil {
		return cancels, _0
	}

	buyCancels, buySideAvailable := sideAvailable(poolStorage, perpetualIndex, computedAccount.MarginBalance, account.PositionAmount, account.TargetLeverage, account.WalletBalance, buyOrders)
	cancels = append(cancels, buyCancels...)
	sellCancels, sellSideAvailable := sideAvailable(poolStorage, perpetualIndex, computedAccount.MarginBalance, account.PositionAmount, account.TargetLeverage, account.WalletBalance, sellOrders)

	cancels = append(cancels, sellCancels...)

	return cancels, decimal.Min(buySideAvailable, sellSideAvailable)
}

func CheckCloseOnly(account *model.AccountStorage, order *model.Order) decimal.Decimal {
	if order.IsCloseOnly {
		if account.PositionAmount.IsZero() {
			// position is 0, cancel all
			return order.AvailableAmount
		} else {
			// open amount has same sign with position, cancel all
			if utils.HasTheSameSign(account.PositionAmount, order.Amount) {
				return order.AvailableAmount
			}

			// closed amount greater than position, cancel some
			if order.AvailableAmount.Abs().GreaterThan(account.PositionAmount.Abs()) {
				return order.AvailableAmount.Add(account.PositionAmount)
			}
		}
	}
	return _0
}

type MatchItem struct {
	Order              *orderbook.MemoryOrder // NOTE: mutable! should only be modified where execute match
	OrderCancelAmounts []decimal.Decimal
	OrderCancelReasons []model.CancelReasonType
	OrderTotalCancel   decimal.Decimal

	MatchedAmount decimal.Decimal
}

func (m *match) MatchOrderSideBySide() []*MatchItem {
	result := make([]*MatchItem, 0)
	bidPrices := m.orderbook.GetBidPricesDesc()
	askPrices := m.orderbook.GetAskPricesAsc()
	bidIdx := 0
	askIdx := 0
	bidContinue := true
	askContinue := true

	if len(bidPrices) == 0 && len(askPrices) == 0 {
		return result
	}
	// compute match orders
	poolStorage := m.poolSyncer.GetPoolStorage(m.perpetual.LiquidityPoolAddress)
	if poolStorage == nil {
		logger.Errorf("MatchOrderSideBySide: GetLiquidityPoolStorage fail!")
		return result
	}

	orderGasLimit := mai3.GetGasFeeLimit(len(poolStorage.Perpetuals), false)
	maiV3MaxMatchGroup := conf.Conf.GasLimit / uint64(orderGasLimit)

	now := time.Now()
	for {
		if len(bidPrices) > bidIdx {
			now = time.Now()
			result, bidContinue = m.matchOneSide(poolStorage, bidPrices[bidIdx], true, result, maiV3MaxMatchGroup)
			logger.Infof("matchOneSide %s-%d cost: %0.4f", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex, time.Since(now).Seconds())

			bidIdx++
		} else {
			bidContinue = false
		}

		if len(askPrices) > askIdx {
			now = time.Now()
			result, askContinue = m.matchOneSide(poolStorage, askPrices[askIdx], false, result, maiV3MaxMatchGroup)
			logger.Infof("matchOneSide %s-%d cost: %0.4f", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex, time.Since(now).Seconds())
			askIdx++
		} else {
			askContinue = false
		}

		if !bidContinue && !askContinue {
			break
		}
	}

	return result
}

func (m *match) matchOneSide(poolStorage *model.LiquidityPoolStorage, tradePrice decimal.Decimal, isBuy bool, result []*MatchItem, maiV3MaxMatchGroup uint64) ([]*MatchItem, bool) {
	orders := make([]*orderbook.MemoryOrder, 0)
	if isBuy {
		orders = append(orders, m.orderbook.GetBidOrdersByPrice(tradePrice)...)
	} else {
		orders = append(orders, m.orderbook.GetAskOrdersByPrice(tradePrice)...)
	}
	if len(orders) == 0 {
		return result, true
	}

	perpetual, ok := poolStorage.Perpetuals[m.perpetual.PerpetualIndex]
	if !ok || !perpetual.IsNormal {
		return result, false
	}

	now := time.Now()
	maxTradeAmount := mai3.ComputeAMMAmountWithPrice(poolStorage, m.perpetual.PerpetualIndex, isBuy, tradePrice)
	logger.Infof("mai3 ComputeAMMAmountWithPrice cost:%0.4f", time.Since(now).Seconds())
	logger.Infof("maxAmount:%s, isBuy:%v, tradePrice:%s perpetual:%s-%d ", maxTradeAmount, isBuy, tradePrice, m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex)
	if maxTradeAmount.IsZero() || !utils.HasTheSameSign(maxTradeAmount, orders[0].Amount) {
		return result, false
	}

	for _, order := range orders {
		if uint64(len(result)) == maiV3MaxMatchGroup {
			return result, false
		}

		logger.Infof("memoryOrder:%+v", order)

		if maxTradeAmount.Abs().LessThan(order.MinTradeAmount.Abs()) {
			continue
		}

		// check stop order
		if order.Type == model.StopLimitOrder || order.Type == model.TakeProfitOrder {
			logger.Infof("indexPrice:%s", perpetual.IndexPrice)
			if perpetual.IndexPrice.IsZero() {
				continue
			}

			if order.Type == model.StopLimitOrder {
				// When amount > 0, if stop loss order: index price must >= trigger price,
				// When amount < 0, if stop loss order: index price must <= trigger price,
				if order.Amount.IsPositive() && perpetual.IndexPrice.LessThan(order.TriggerPrice) {
					continue
				} else if order.Amount.IsNegative() && perpetual.IndexPrice.GreaterThan(order.TriggerPrice) {
					continue
				}
			} else {
				// When amount > 0, if take profit order: index price must <= trigger price,
				// When amount < 0, if take profit order: index price must >= trigger price,
				if order.Amount.IsPositive() && perpetual.IndexPrice.GreaterThan(order.TriggerPrice) {
					continue
				} else if order.Amount.IsNegative() && perpetual.IndexPrice.LessThan(order.TriggerPrice) {
					continue
				}
			}
		}

		account, err := m.chainCli.GetAccountStorage(m.ctx, conf.Conf.ReaderAddress, m.perpetual.PerpetualIndex, m.perpetual.LiquidityPoolAddress, order.Trader)
		if account == nil || err != nil {
			logger.Errorf("matchOneSide: GetAccountStorage fail! err:%v", err)
			return result, false
		}

		balance, err := m.chainCli.BalanceOf(m.ctx, m.perpetual.CollateralAddress, order.Trader, m.perpetual.CollateralDecimals)
		if err != nil {
			logger.Errorf("matchOneSide: BalanceOf fail! err:%v", err)
			return result, false
		}
		allowance, err := m.chainCli.Allowance(m.ctx, m.perpetual.CollateralAddress, order.Trader, m.perpetual.LiquidityPoolAddress, m.perpetual.CollateralDecimals)
		if err != nil {
			logger.Errorf("matchOneSide: Allowance fail! err:%v", err)
			return result, false
		}
		account.WalletBalance = decimal.Min(balance, allowance)

		if maxTradeAmount.Abs().GreaterThanOrEqual(order.Amount.Abs()) {
			matchItem := &MatchItem{
				Order:              order,
				OrderCancelAmounts: make([]decimal.Decimal, 0),
				OrderCancelReasons: make([]model.CancelReasonType, 0),
				OrderTotalCancel:   decimal.Zero,
				MatchedAmount:      order.Amount,
			}
			logger.Infof("matchedAmount: %s orderAmount:%s", order.Amount, order.Amount)
			_, tradeIsSafe, _, err := mai3.ComputeAMMTrade(poolStorage, m.perpetual.PerpetualIndex, account, order.Amount)
			if err != nil || !tradeIsSafe {
				logger.Infof("matchOneSide: ComputeAMMTrade fail or unsafe after trade. err:%s", err)
				// unsafe after trade, try to get max trade amount
				amount := mai3.ComputeAMMMaxTradeAmount(poolStorage, m.perpetual.PerpetualIndex, account, order.Amount, isBuy)
				if amount.IsZero() || amount.Abs().LessThan(order.MinTradeAmount) {
					continue
				}
				_, tradeIsSafe, _, err := mai3.ComputeAMMTrade(poolStorage, m.perpetual.PerpetualIndex, account, amount)
				if err != nil || !tradeIsSafe {
					continue
				}
				matchItem.MatchedAmount = amount
				// check remain amount bigger than min trade amount
				if order.Amount.Sub(amount).Abs().LessThan(order.MinTradeAmount.Abs()) {
					logger.Infof("OrderCancelAmount: %s", order.Amount.Sub(amount))
					matchItem.OrderCancelAmounts = append(matchItem.OrderCancelAmounts, order.Amount.Sub(amount))
					matchItem.OrderCancelReasons = append(matchItem.OrderCancelReasons, model.CancelReasonRemainTooSmall)
					matchItem.OrderTotalCancel = order.Amount.Sub(amount)
				}
			}

			result = append(result, matchItem)
			maxTradeAmount = maxTradeAmount.Sub(order.Amount)
		} else {
			matchedAmount := maxTradeAmount.Mul(TradeAmountRelaxFactor).Round(mai3.DECIMALS)
			matchItem := &MatchItem{
				Order:              order,
				OrderCancelAmounts: make([]decimal.Decimal, 0),
				OrderCancelReasons: make([]model.CancelReasonType, 0),
				OrderTotalCancel:   decimal.Zero,
				MatchedAmount:      matchedAmount,
			}
			logger.Infof("matchedAmount: %s orderAmount:%s", matchedAmount, order.Amount)
			_, tradeIsSafe, _, err := mai3.ComputeAMMTrade(poolStorage, m.perpetual.PerpetualIndex, account, matchedAmount)
			if err != nil || !tradeIsSafe {
				logger.Infof("matchOneSide: ComputeAMMTrade fail or unsafe after trade. err:%v", err)
				// unsafe after trade, try to get max trade amount
				amount := mai3.ComputeAMMMaxTradeAmount(poolStorage, m.perpetual.PerpetualIndex, account, order.Amount, isBuy)
				if amount.IsZero() || amount.Abs().LessThan(order.MinTradeAmount) {
					continue
				}
				_, tradeIsSafe, _, err := mai3.ComputeAMMTrade(poolStorage, m.perpetual.PerpetualIndex, account, amount)
				if err != nil || !tradeIsSafe {
					continue
				}
				matchItem.MatchedAmount = amount
			}
			if order.Amount.Sub(matchedAmount).Abs().LessThan(order.MinTradeAmount.Abs()) {
				logger.Infof("OrderCancelAmount: %s", order.Amount.Sub(matchedAmount))
				matchItem.OrderCancelAmounts = append(matchItem.OrderCancelAmounts, order.Amount.Sub(matchedAmount))
				matchItem.OrderCancelReasons = append(matchItem.OrderCancelReasons, model.CancelReasonRemainTooSmall)
				matchItem.OrderTotalCancel = order.Amount.Sub(matchedAmount)
			}
			result = append(result, matchItem)
			break
		}
	}

	return result, true
}
