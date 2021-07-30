package match

import (
	"context"
	"fmt"
	"time"

	"github.com/mcdexio/mai3-broker/common/message"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/shopspring/decimal"
	"gopkg.in/guregu/null.v3"

	logger "github.com/sirupsen/logrus"
)

const TriggerPriceNotReach = "trigger price is not reached"
const PriceExceedsLimit = "price exceeds limit"

func (m *match) UpdateOrdersStatus(txID string, status model.TransactionStatus, transactionHash, blockHash string, blockNumber, blockTime uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ordersToNotify := make([]*model.Order, 0)
	err := m.dao.Transaction(context.Background(), false /* readonly */, func(dao dao.DAO) error {
		// update match_transaction
		matchTx, err := dao.GetMatchTransaction(txID)
		if err != nil {
			return err
		}
		matchTx.BlockConfirmed = true
		matchTx.Status = status
		if (matchTx.Status == model.TransactionStatusFail || matchTx.Status == model.TransactionStatusSuccess) && blockNumber > 0 {
			matchTx.TransactionHash = null.StringFrom(transactionHash)
			matchTx.BlockHash = null.StringFrom(blockHash)
			matchTx.BlockNumber = null.IntFrom(int64(blockNumber))
			matchTx.ExecutedAt = null.TimeFrom(time.Unix(int64(blockTime), 0).UTC())
		}

		mTxPendingDuration.WithLabelValues(fmt.Sprintf("%s-%d", matchTx.LiquidityPoolAddress, matchTx.PerpetualIndex)).Set(float64(time.Since(matchTx.CreatedAt).Milliseconds()))

		// update orders
		orders, err := m.updateOrdersByTradeEvent(dao, matchTx, blockNumber)
		if err != nil {
			return err
		}
		if err = dao.UpdateMatchTransaction(matchTx); err != nil {
			return err
		}

		ordersToNotify = append(ordersToNotify, orders...)
		return nil
	})

	if err == nil {
		for _, order := range ordersToNotify {
			// notice websocket for order change
			wsMsg := message.WebSocketMessage{
				ChannelID: message.GetAccountChannelID(order.TraderAddress),
				Payload: message.WebSocketOrderChangePayload{
					Type:            message.WsTypeOrderChange,
					Order:           order,
					BlockNumber:     blockNumber,
					TransactionHash: transactionHash,
				},
			}
			m.wsChan <- wsMsg
		}
	}
	return err
}

func (m *match) updateOrdersByTradeEvent(dao dao.DAO, matchTx *model.MatchTransaction, blockNumber uint64) ([]*model.Order, error) {
	ordersToNotify := make([]*model.Order, 0)
	ctxTimeout, ctxTimeoutCancel := context.WithTimeout(m.ctx, conf.Conf.ChainTimeout)
	defer ctxTimeoutCancel()
	// orders trade success
	orderSuccMap := make(map[string]decimal.Decimal)
	// orders trade failed, need to be cancel
	orderFailMap := make(map[string]decimal.Decimal)

	orderMatchMap := make(map[string]decimal.Decimal)
	orderHashes := make([]string, 0)

	if matchTx.Status == model.TransactionStatusSuccess {
		tradeSuccess, err := m.chainCli.FilterTradeSuccess(ctxTimeout, matchTx.BrokerAddress, blockNumber, blockNumber)
		if err != nil {
			return ordersToNotify, err
		}
		TradeFailed, err := m.chainCli.FilterTradeFailed(ctxTimeout, matchTx.BrokerAddress, blockNumber, blockNumber)
		if err != nil {
			return ordersToNotify, err
		}

		for _, event := range tradeSuccess {
			logger.Infof("Trade Success: %+v", event)
			if event.TransactionHash != matchTx.TransactionHash.String {
				continue
			}
			matchInfo := &model.MatchItem{
				OrderHash: event.OrderHash,
				Amount:    event.Amount,
			}
			matchTx.MatchResult.SuccItems = append(matchTx.MatchResult.SuccItems, matchInfo)
			orderSuccMap[event.OrderHash] = event.Amount
		}

		for _, event := range TradeFailed {
			logger.Infof("Trade Failed: %+v", event)
			if event.TransactionHash != matchTx.TransactionHash.String {
				continue
			}
			// trigger price or price not match in contract, will rollback to orderbook
			if event.Reason == TriggerPriceNotReach || event.Reason == PriceExceedsLimit {
				continue
			}
			matchInfo := &model.MatchItem{
				OrderHash: event.OrderHash,
				Amount:    event.Amount,
			}
			matchTx.MatchResult.FailedItems = append(matchTx.MatchResult.FailedItems, matchInfo)
			orderFailMap[event.OrderHash] = event.Amount
		}
	}

	for _, item := range matchTx.MatchResult.MatchItems {
		orderMatchMap[item.OrderHash] = item.Amount
		orderHashes = append(orderHashes, item.OrderHash)
	}

	orders, err := dao.GetOrderByHashs(orderHashes)
	if err != nil {
		logger.Errorf("UpdateOrdersStatus:%s", err)
		return ordersToNotify, err
	}
	for _, order := range orders {
		// order success
		if amount, ok := orderSuccMap[order.OrderHash]; ok {
			// partial success
			matchAmount := orderMatchMap[order.OrderHash]
			if !amount.Equal(matchAmount) {
				oldAmount := order.AvailableAmount
				order.AvailableAmount = order.AvailableAmount.Add(matchAmount.Sub(amount))
				if err := m.rollbackOrderbook(oldAmount, order.PendingAmount.Sub(amount), order); err != nil {
					logger.Errorf("UpdateOrdersStatus:%s", err)
					return ordersToNotify, err
				}
			}
			order.PendingAmount = order.PendingAmount.Sub(matchAmount)
			order.ConfirmedAmount = order.ConfirmedAmount.Add(amount)
			if err := dao.UpdateOrder(order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}
		} else if amount, ok := orderFailMap[order.OrderHash]; ok {
			// order failed, cancel order
			order.PendingAmount = order.PendingAmount.Sub(amount)
			order.CanceledAmount = order.CanceledAmount.Add(amount)
			r := &model.OrderCancelReason{
				Reason:          model.CancelReasonTransactionFail,
				Amount:          amount,
				TransactionHash: matchTx.TransactionHash.String,
				CanceledAt:      matchTx.ExecutedAt.Time,
			}
			order.CancelReasons = append(order.CancelReasons, r)
			if err := dao.UpdateOrder(order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}
		} else {
			// order not excute, reload order in orderbook
			matchAmount := orderMatchMap[order.OrderHash]
			oldAmount := order.AvailableAmount
			order.PendingAmount = order.PendingAmount.Sub(matchAmount)
			order.AvailableAmount = order.AvailableAmount.Add(matchAmount)
			if err := dao.UpdateOrder(order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}

			if err := m.rollbackOrderbook(oldAmount, matchAmount, order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}
		}
		ordersToNotify = append(ordersToNotify, order)
	}
	return ordersToNotify, nil
}

func (m *match) rollbackOrderbook(oldAmount, delta decimal.Decimal, order *model.Order) error {
	if oldAmount.IsZero() {
		memoryOrder := m.getMemoryOrder(order)
		if err := m.orderbook.InsertOrder(memoryOrder); err != nil {
			logger.Errorf("insert order to orderbook:%v", err)
			return err
		}
		return nil
	}

	bookOrder, ok := m.orderbook.GetOrder(order.OrderHash, order.Amount.IsNegative(), order.Price)
	if ok {
		if err := m.orderbook.ChangeOrder(bookOrder, delta); err != nil {
			logger.Errorf("change order in orderbook:%v", err)
			return err
		}
		return nil
	}
	return fmt.Errorf("order %s not fund in orderbook", order.OrderHash)
}

func (m *match) RollbackOrdersStatus(txID string, status model.TransactionStatus, transactionHash, blockHash string, blockNumber, blockTime uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ordersToNotify := make([]*model.Order, 0)
	err := m.dao.Transaction(context.Background(), false /* readonly */, func(dao dao.DAO) error {
		// update match_transaction
		matchTx, err := dao.GetMatchTransaction(txID)
		if err != nil {
			return err
		}
		matchTx.TransactionHash = null.StringFrom(transactionHash)
		matchTx.BlockHash = null.StringFrom(blockHash)
		matchTx.BlockNumber = null.IntFrom(int64(blockNumber))
		matchTx.ExecutedAt = null.TimeFrom(time.Unix(int64(blockTime), 0).UTC())

		if matchTx.Status == model.TransactionStatusSuccess && status == model.TransactionStatusFail {
			orders, err := m.rollbackOrdersOnTransactionFail(dao, matchTx)
			if err != nil {
				return err
			}
			ordersToNotify = append(ordersToNotify, orders...)
		} else if matchTx.Status == model.TransactionStatusFail && status == model.TransactionStatusSuccess {
			orders, err := m.updateOrdersByTradeEvent(dao, matchTx, blockNumber)
			if err != nil {
				return err
			}
			ordersToNotify = append(ordersToNotify, orders...)
		}
		matchTx.Status = status
		if err = dao.UpdateMatchTransaction(matchTx); err != nil {
			return err
		}

		return nil
	})

	if err == nil {
		for _, order := range ordersToNotify {
			// notice websocket for order change
			wsMsg := message.WebSocketMessage{
				ChannelID: message.GetAccountChannelID(order.TraderAddress),
				Payload: message.WebSocketOrderChangePayload{
					Type:  message.WsTypeOrderChange,
					Order: order,
				},
			}
			m.wsChan <- wsMsg
		}
	}
	return err
}

func (m *match) rollbackOrdersOnTransactionFail(dao dao.DAO, matchTx *model.MatchTransaction) ([]*model.Order, error) {
	ordersToNotify := make([]*model.Order, 0)
	orderRollbackMap := make(map[string]decimal.Decimal)
	orderHashes := make([]string, 0)
	// success orders
	for _, item := range matchTx.MatchResult.SuccItems {
		orderRollbackMap[item.OrderHash] = item.Amount
		orderHashes = append(orderHashes, item.OrderHash)
	}

	orders, err := dao.GetOrderByHashs(orderHashes)
	if err != nil {
		logger.Errorf("rollbackOrdersOnTransactionFail:%v", err)
		return ordersToNotify, err
	}
	for _, order := range orders {
		amount := orderRollbackMap[order.OrderHash]
		oldAmount := order.AvailableAmount
		order.PendingAmount = order.ConfirmedAmount.Sub(amount)
		order.AvailableAmount = order.AvailableAmount.Add(amount)
		if err := dao.UpdateOrder(order); err != nil {
			logger.Errorf("rollbackOrdersOnTransactionFail:%v", err)
			return ordersToNotify, err
		}

		if err := m.rollbackOrderbook(oldAmount, amount, order); err != nil {
			logger.Errorf("rollbackOrdersOnTransactionFail:%v", err)
			return ordersToNotify, err
		}
		ordersToNotify = append(ordersToNotify, order)
	}

	return ordersToNotify, nil
}
