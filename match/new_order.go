package match

import (
	"fmt"

	"github.com/mcdexio/mai3-broker/common/mai3"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/message"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

type OrdersEachPerp struct {
	LiquidityPoolAddress string
	PerpetualIndex       int64
	PoolStorage          *model.LiquidityPoolStorage
	Orders               []*model.Order
}

func (m *match) splitActiveOrdersWithCollateral(orders []*model.Order, collateral string) (map[string]*OrdersEachPerp, error) {
	res := make(map[string]*OrdersEachPerp)
	for _, order := range orders {
		// orders split in different perpetual
		perpetualID := fmt.Sprintf("%s-%d", order.LiquidityPoolAddress, order.PerpetualIndex)
		ordersEachPerp, ok := res[perpetualID]
		if ok {
			ordersEachPerp.Orders = append(ordersEachPerp.Orders, order)
		} else {
			// use the same collateral
			if order.CollateralAddress == collateral {
				poolStorage := m.poolSyncer.GetPoolStorage(order.LiquidityPoolAddress)
				if poolStorage == nil {
					return res, fmt.Errorf("get pool storage error")
				}
				res[perpetualID] = &OrdersEachPerp{
					LiquidityPoolAddress: order.LiquidityPoolAddress,
					PerpetualIndex:       order.PerpetualIndex,
					PoolStorage:          poolStorage,
					Orders:               []*model.Order{order},
				}
			}
		}
	}

	return res, nil
}

func (m *match) NewOrder(order *model.Order) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// get all pending orders (multiple perpetuals maybe)
	activeOrders, err := m.dao.QueryOrder(order.TraderAddress, "", 0, []model.OrderStatus{model.OrderPending}, 0, 0, 0)
	if err != nil {
		logger.Errorf("new order: QueryOrder err:%s", err)
		return model.MatchInternalErrorID
	}

	if len(activeOrders) >= conf.Conf.MaxOrderNum {
		return model.MatchMaxOrderNumReachID
	}

	account, err := m.chainCli.GetAccountStorage(m.ctx, conf.Conf.ReaderAddress, m.perpetual.PerpetualIndex, m.perpetual.LiquidityPoolAddress, order.TraderAddress)
	if account == nil || err != nil {
		logger.Errorf("new order:GetAccountStorage err:%v", err)
		return model.MatchInternalErrorID
	}

	balance, err := m.chainCli.BalanceOf(m.ctx, m.perpetual.CollateralAddress, order.TraderAddress, m.perpetual.CollateralDecimals)
	if err != nil {
		logger.Errorf("new order:BalanceOf err:%v", err)
		return model.MatchInternalErrorID
	}
	allowance, err := m.chainCli.Allowance(m.ctx, m.perpetual.CollateralAddress, order.TraderAddress, m.perpetual.LiquidityPoolAddress, m.perpetual.CollateralDecimals)
	if err != nil {
		logger.Errorf("new order:Allowance err:%v", err)
		return model.MatchInternalErrorID
	}
	walletBalance := decimal.Min(balance, allowance)

	if !CheckCloseOnly(account, order).Equal(_0) {
		return model.MatchCloseOnlyErrorID
	}

	// get from cache
	poolStorage := m.poolSyncer.GetPoolStorage(m.perpetual.LiquidityPoolAddress)
	if poolStorage == nil {
		logger.Errorf("new order: GetLiquidityPoolStorage fail! err:%v", err)
		return model.MatchInternalErrorID
	}

	perpetual, ok := poolStorage.Perpetuals[m.perpetual.PerpetualIndex]
	if !ok || !perpetual.IsNormal {
		logger.Errorf("new order: perpetual status is not normal!")
		return model.MatchInternalErrorID
	}

	// check gas
	order.GasFeeLimit = mai3.GetGasFeeLimit(len(poolStorage.Perpetuals), order.IsCloseOnly)
	if conf.Conf.GasEnable {
		gasBalance, err := m.chainCli.GetGasBalance(m.ctx, conf.Conf.BrokerAddress, order.TraderAddress)
		if err != nil {
			logger.Errorf("new order: checkUserPendingOrders:%v", err)
			return model.MatchInternalErrorID
		}
		gasBalance = utils.ToWad(gasBalance)
		gasPrice := m.gasMonitor.GasPriceInWei()
		gasReward := gasPrice.Mul(decimal.NewFromInt(order.GasFeeLimit))
		// gasReward := utils.ToWad(decimal.NewFromFloat(0.0001))
		ordersGasReward := gasReward.Mul(decimal.NewFromInt(int64(len(activeOrders) + 1)))
		logger.Infof("gasPrice:%s brokerFeeLimit:%s gasBalance:%s gasReward:%s ordersGasReword:%s", gasPrice, utils.ToGwei(decimal.NewFromInt(order.BrokerFeeLimit)), gasBalance, gasReward, ordersGasReward)
		if gasBalance.LessThan(ordersGasReward) {
			return model.MatchGasNotEnoughErrorID
		}

		if utils.ToGwei(decimal.NewFromInt(order.BrokerFeeLimit)).LessThan(gasReward) {
			return model.MatchGasNotEnoughErrorID
		}
	}

	activeOrders = append(activeOrders, order)
	orderMaps, err := m.splitActiveOrdersWithCollateral(activeOrders, order.CollateralAddress)
	if err != nil {
		return model.MatchInternalErrorID
	}

	for _, v := range orderMaps {
		var a *model.AccountStorage
		// get account storage
		if v.LiquidityPoolAddress == m.perpetual.LiquidityPoolAddress && v.PerpetualIndex == m.perpetual.PerpetualIndex {
			a = account
		} else {
			a, err = m.chainCli.GetAccountStorage(m.ctx, conf.Conf.ReaderAddress, v.PerpetualIndex, v.LiquidityPoolAddress, order.TraderAddress)
			if a == nil || err != nil {
				logger.Errorf("new order:GetAccountStorage err:%v", err)
				return model.MatchInternalErrorID
			}
		}
		a.WalletBalance = walletBalance
		// TODO: consider cancel orders
		_, available := ComputeOrderAvailable(v.PoolStorage, v.PerpetualIndex, a, v.Orders)
		// available balance less than 0, InsufficientBalance
		if available.LessThan(_0) {
			return model.MatchInsufficientBalanceErrorID
		}
		walletBalance = available
	}

	// create order and insert to db and orderbook
	err = m.dao.Transaction(m.ctx, false /* readonly */, func(dao dao.DAO) error {
		if err := dao.CreateOrder(order); err != nil {
			return err
		}

		memoryOrder := m.getMemoryOrder(order)
		if err := m.orderbook.InsertOrder(memoryOrder); err != nil {
			return err
		}
		if err := m.setExpirationTimer(order.OrderHash, order.ExpiresAt); err != nil {
			return err
		}
		return nil
	})

	if err == nil {
		// notice websocket for new order
		wsMsg := message.WebSocketMessage{
			ChannelID: message.GetAccountChannelID(order.TraderAddress),
			Payload: message.WebSocketOrderChangePayload{
				Type:  message.WsTypeOrderChange,
				Order: order,
			},
		}
		m.wsChan <- wsMsg
		// trigger match order
		if len(m.trgr) < ChannelHWM {
			m.trgr <- nil
		}
		return model.MatchOK
	}
	logger.Errorf("new order: %s", err)
	return model.MatchInternalErrorID
}
