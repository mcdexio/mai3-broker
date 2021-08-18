package match

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/mcdexio/mai3-broker/common/chain"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/message"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/common/orderbook"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/mcdexio/mai3-broker/gasmonitor"
	logger "github.com/sirupsen/logrus"

	"sync"
	"time"
)

var ChannelHWM = 64

type match struct {
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	poolSyncer *poolSyncer
	wsChan     chan interface{}
	orderbook  *orderbook.Orderbook
	perpetual  *model.Perpetual
	chainCli   chain.ChainClient
	gasMonitor *gasmonitor.GasMonitor
	dao        dao.DAO
	timerMu    sync.Mutex
	timers     map[string]*time.Timer
	trgr       chan interface{}
}

func newMatch(ctx context.Context, cli chain.ChainClient, dao dao.DAO, poolSyncer *poolSyncer, perpetual *model.Perpetual, wsChan chan interface{}, gm *gasmonitor.GasMonitor) (*match, error) {
	ctx, cancel := context.WithCancel(ctx)
	m := &match{
		ctx:        ctx,
		cancel:     cancel,
		poolSyncer: poolSyncer,
		wsChan:     wsChan,
		perpetual:  perpetual,
		orderbook:  orderbook.NewOrderbook(),
		chainCli:   cli,
		gasMonitor: gm,
		dao:        dao,
		timers:     make(map[string]*time.Timer),
		trgr:       make(chan interface{}, ChannelHWM),
	}

	if err := m.reloadActiveOrders(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *match) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopTimers()
	m.cancel()
}

func (m *match) GetCollateralDecimal() int32 {
	return m.perpetual.CollateralDecimals
}

func (m *match) matchOrders() {
	m.mu.Lock()
	defer m.mu.Unlock()

	orders, err := m.dao.QueryOrder("", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex, []model.OrderStatus{model.OrderPending}, 0, 0, 0)
	if err != nil {
		return
	}

	// update active orders count in metrics
	activeOrderCount.WithLabelValues(fmt.Sprintf("%s-%d", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex)).Set(float64(len(orders)))
	if len(orders) == 0 {
		return
	}

	// transactions, err := m.dao.QueryUnconfirmedTransactionsByContract(m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex)
	// if err != nil {
	// 	logger.Errorf("Match: QueryUnconfirmedTransactionsByContract failed perpetual:%s-%d error:%s", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex, err.Error())
	// 	return
	// }
	// if len(transactions) > 0 {
	// 	logger.Infof("Match: unconfirmed transaction exists. wait for it to be confirmed perpetual:%s-%d", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex)
	// 	return
	// }

	now := time.Now()
	matchItems := m.MatchOrderSideBySide()
	logger.Infof("matchOrders %s-%d MatchOrderSideBySide cost: %0.4f", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex, time.Since(now).Seconds())

	if len(matchItems) == 0 {
		return
	}

	u, err := uuid.NewRandom()
	if err != nil {
		logger.Errorf("generate transaction uuid error:%s", err.Error())
		return
	}
	matchTransaction := &model.MatchTransaction{
		ID:                   u.String(),
		Status:               model.TransactionStatusInit,
		LiquidityPoolAddress: m.perpetual.LiquidityPoolAddress,
		PerpetualIndex:       m.perpetual.PerpetualIndex,
		BrokerAddress:        conf.Conf.BrokerAddress,
	}
	ordersForNotify := make([]*model.Order, 0)
	err = m.dao.Transaction(m.ctx, false /* readonly */, func(dao dao.DAO) error {
		for _, item := range matchItems {
			order, err := dao.GetOrder(item.Order.ID)
			if err != nil {
				return fmt.Errorf("get Order[%s] failed, error:%w", item.Order.ID, err)
			}
			newAmount := order.AvailableAmount.Sub(item.MatchedAmount)
			if !utils.HasTheSameSign(newAmount, order.AvailableAmount) {
				return fmt.Errorf("order[%s] avaliable is negative after match", order.OrderHash)
			}
			order.AvailableAmount = newAmount
			order.PendingAmount = order.PendingAmount.Add(item.MatchedAmount)
			if !item.OrderTotalCancel.IsZero() {
				order.CanceledAmount = order.CanceledAmount.Add(item.OrderTotalCancel)
				order.AvailableAmount = order.AvailableAmount.Sub(item.OrderTotalCancel)
				for i := 0; i < len(item.OrderCancelAmounts); i++ {
					order.CancelReasons = append(order.CancelReasons, &model.OrderCancelReason{
						Reason:     item.OrderCancelReasons[i],
						Amount:     item.OrderCancelAmounts[i],
						CanceledAt: time.Now().UTC(),
					})
				}
			}
			matchTransaction.MatchResult.MatchItems = append(matchTransaction.MatchResult.MatchItems, &model.MatchItem{
				OrderHash: order.OrderHash,
				Amount:    item.MatchedAmount,
			})
			if err := dao.UpdateOrder(order); err != nil {
				return fmt.Errorf("order[%s] update failed error:%w", order.OrderHash, err)
			}
			ordersForNotify = append(ordersForNotify, order)
		}
		if err := dao.CreateMatchTransaction(matchTransaction); err != nil {
			return fmt.Errorf("matchTransaction create failed error:%w", err)
		}

		// update memory order after all db operations success
		for _, item := range matchItems {
			if err := m.orderbook.ChangeOrder(item.Order, item.MatchedAmount.Add(item.OrderTotalCancel).Neg()); err != nil {
				return fmt.Errorf("order[%s] orderbook ChangeOrder failed error:%w", item.Order.ID, err)
			}
		}
		return nil
	})

	if err == nil {
		// notice websocket for order change
		for _, order := range ordersForNotify {
			wsMsg := message.WebSocketMessage{
				ChannelID: message.GetAccountChannelID(order.TraderAddress),
				Payload: message.WebSocketOrderChangePayload{
					Type:  message.WsTypeOrderChange,
					Order: order,
				},
			}
			m.wsChan <- wsMsg
		}
	} else {
		logger.Errorf("match orders fail. error:%s", err)
	}

}

func (m *match) RunMatch(ctx context.Context) error {
	// logger.Infof("match perpetual:%s-%d matchOrders start", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex)
	var (
		before  time.Time
		elasped time.Duration
		timer   = time.NewTimer(0)
	)
	for {
		select {
		case <-ctx.Done():
			// logger.Infof("match perpetual:%s-%d matchOrders end", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex)
			return nil
		case <-m.trgr:
		case <-timer.C:
		}
		before = time.Now()
		m.matchOrders()
		elasped = time.Since(before)
		if elasped >= conf.Conf.MatchInterval {
			timer.Reset(time.Second)
		} else {
			timer.Reset(conf.Conf.MatchInterval - elasped)
		}
	}
}

func (m *match) reloadActiveOrders() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	orders, err := m.dao.QueryOrder("", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex, []model.OrderStatus{model.OrderPending}, 0, 0, 0)
	if err != nil {
		return err
	}

	activeOrderCount.WithLabelValues(fmt.Sprintf("%s-%d", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex)).Set(float64(len(orders)))
	for _, order := range orders {
		if !order.AvailableAmount.IsZero() {
			memoryOrder := m.getMemoryOrder(order)
			if err := m.orderbook.InsertOrder(memoryOrder); err != nil {
				return fmt.Errorf("reloadActiveOrders:%w", err)
			}

			if err := m.setExpirationTimer(order.OrderHash, order.ExpiresAt); err != nil {
				return fmt.Errorf("reloadActiveOrders:%w", err)
			}
		}
	}
	return nil
}

func (m *match) getMemoryOrder(order *model.Order) *orderbook.MemoryOrder {
	return &orderbook.MemoryOrder{
		ID:                   order.OrderHash,
		LiquidityPoolAddress: order.LiquidityPoolAddress,
		PerpetualIndex:       order.PerpetualIndex,
		Price:                order.Price,
		TriggerPrice:         order.TriggerPrice,
		Amount:               order.AvailableAmount,
		MinTradeAmount:       order.MinTradeAmount,
		Type:                 order.Type,
		Trader:               order.TraderAddress,
	}
}
