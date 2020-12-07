package match

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/mcarloai/mai-v3-broker/common/chain"
	"github.com/mcarloai/mai-v3-broker/common/model"
	"github.com/mcarloai/mai-v3-broker/common/orderbook"
	"github.com/mcarloai/mai-v3-broker/conf"
	"github.com/mcarloai/mai-v3-broker/dao"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type match struct {
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	wsChan    chan interface{}
	orderbook *orderbook.Orderbook
	stopbook  *orderbook.Orderbook
	perpetual *model.Perpetual
	chainCli  chain.ChainClient
	dao       dao.DAO
	timers    map[string]*time.Timer
}

func newMatch(ctx context.Context, cli chain.ChainClient, dao dao.DAO, perpetual *model.Perpetual, wsChan chan interface{}) (*match, error) {
	ctx, cancel := context.WithCancel(ctx)
	m := &match{
		ctx:       ctx,
		cancel:    cancel,
		wsChan:    wsChan,
		perpetual: perpetual,
		orderbook: orderbook.NewOrderbook(),
		stopbook:  orderbook.NewOrderbook(),
		chainCli:  cli,
		dao:       dao,
		timers:    make(map[string]*time.Timer),
	}
	m.run()

	return m, nil
}

func (m *match) run() error {
	if err := m.reloadActiveOrders(); err != nil {
		return err
	}

	// go monitor check user margin gas
	go m.checkOrdersMargin()

	// go match order
	go m.runMatch()

	return nil
}

func (m *match) checkOrdersMargin() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(10 * time.Second):
			// TODO
			// check margin
			// check gas
		}
	}
}

func (m *match) matchStopOrders(indexPrice decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		for {
			minBidPrice := m.stopbook.MinBid()
			if minBidPrice != nil && minBidPrice.LessThanOrEqual(indexPrice) {
				orders := m.stopbook.GetBidOrdersByPrice(*minBidPrice)
				for _, order := range orders {
					m.changeStopOrder(order)
				}
			} else {
				break
			}
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for {
			MaxAskPrice := m.stopbook.MaxAsk()
			if MaxAskPrice != nil && MaxAskPrice.GreaterThanOrEqual(indexPrice) {
				orders := m.stopbook.GetAskOrdersByPrice(*MaxAskPrice)
				for _, order := range orders {
					m.changeStopOrder(order)
				}
			} else {
				break
			}
		}
		wg.Done()
	}()

	wg.Wait()
	return
}

func (m *match) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopTimers()
	m.cancel()
}

func (m *match) changeStopOrder(memoryOrder *orderbook.MemoryOrder) {
	err := m.dao.Transaction(func(dao dao.DAO) error {
		dao.ForUpdate()
		order, err := dao.GetOrder(memoryOrder.ID)
		if err != nil {
			return err
		}
		order.Status = model.OrderPending
		if err = dao.UpdateOrder(order); err != nil {
			return err
		}

		if err := m.stopbook.RemoveOrder(memoryOrder); err != nil {
			return err
		}

		memoryOrder.ComparePrice = memoryOrder.Price
		if err := m.orderbook.InsertOrder(memoryOrder); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.Errorf("change stop order to pending order fail! err:%s", err)
	}
}

func (m *match) matchOrders(indexPrice decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	transactions, err := m.dao.QueryUnconfirmedTransactionsByContract(m.perpetual.PerpetualAddress)
	if err != nil {
		logger.Errorf("Match: QueryUnconfirmedTransactionsByContract failed perpetual:%s error:%s", m.perpetual.PerpetualAddress, err.Error())
		return
	}
	if len(transactions) > 0 {
		logger.Errorf("Match: unconfirmed transaction exists. wait for it to be confirmed perpetual:%s", m.perpetual.PerpetualAddress)
		return
	}
	//TODO
	// 1. compute match orders by index price
	matchItems := m.orderbook.MatchOrder(indexPrice)
	if len(matchItems) == 0 {
		return
	}

	u, err := uuid.NewRandom()
	if err != nil {
		logger.Errorf("generate transaction uuid error:%s", err.Error())
	}
	matchTransaction := &model.MatchTransaction{
		ID:               u.String(),
		Status:           model.TransactionStatusInit,
		PerpetualAddress: m.perpetual.PerpetualAddress,
		BrokerAddress:    conf.Conf.BrokerAddress,
	}
	err = m.dao.Transaction(func(dao dao.DAO) error {
		dao.ForUpdate()
		for _, item := range matchItems {
			order, err := dao.GetOrder(item.Order.ID)
			if err != nil {
				return fmt.Errorf("Match: Get Order[%s] failed, error:%w", item.Order.ID, err)
			}
			newAmount := order.AvailableAmount.Sub(item.MatchedAmount)
			if newAmount.IsNegative() {
				return fmt.Errorf("Match: order[%s] avaliable is negative after match", order.OrderHash)
			}
			order.AvailableAmount = newAmount
			order.PendingAmount = order.PendingAmount.Add(item.MatchedAmount)
			matchTransaction.MatchResult.MatchItems = append(matchTransaction.MatchResult.MatchItems, &model.MatchItem{
				OrderHash: order.OrderHash,
				Amount:    item.MatchedAmount,
			})
			if err := dao.UpdateOrder(order); err != nil {
				return fmt.Errorf("Match: order[%s] update failed error:%w", order.OrderHash, err)
			}
			if err := m.orderbook.ChangeOrder(item.Order, item.MatchedAmount.Neg()); err != nil {
				return fmt.Errorf("Match: order[%s] orderbook ChangeOrder failed error:%w", order.OrderHash, err)
			}
		}
		if err := dao.CreateMatchTransaction(matchTransaction); err != nil {
			return fmt.Errorf("Match: matchTransaction create failed error:%w", err)
		}
		return nil
	})

	if err == nil {
		// 4. send ws msg
	}

	return
}

func (m *match) runMatch() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(time.Second):
			ctxTimeout, ctxTimeoutCancel := context.WithTimeout(m.ctx, conf.Conf.BlockChain.Timeout.Duration)
			defer ctxTimeoutCancel()
			storage, err := m.chainCli.GetPerpetualStorage(ctxTimeout, m.perpetual.PerpetualAddress)
			if err != nil {
				logger.Errorf("get index price fail! err:%s", err.Error())
				continue
			}
			m.matchStopOrders(storage.IndexPrice)
			m.matchOrders(storage.MarkPrice)
		}
	}
}

func (m *match) reloadActiveOrders() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	orders, err := m.dao.QueryOrder("", m.perpetual.PerpetualAddress, []model.OrderStatus{model.OrderPending, model.OrderStop}, 0, 0, 0)
	if err != nil {
		return err
	}
	for _, order := range orders {
		if !order.AvailableAmount.IsZero() {
			memoryOrder := m.getMemoryOrder(order)
			if order.Type == model.StopLimitOrder {
				if err := m.stopbook.InsertOrder(memoryOrder); err != nil {
					return fmt.Errorf("reloadActiveOrders:%w", err)
				}
			} else {
				if err := m.orderbook.InsertOrder(memoryOrder); err != nil {
					return fmt.Errorf("reloadActiveOrders:%w", err)
				}
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
		ID:               order.OrderHash,
		PerpetualAddress: order.PerpetualAddress,
		Price:            order.Price,
		StopPrice:        order.StopPrice,
		Amount:           order.Amount,
		Type:             order.Type,
		Trader:           order.TraderAddress,
	}
}
