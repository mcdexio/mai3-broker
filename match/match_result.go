package match

import (
	"fmt"
	"time"

	"github.com/mcdexio/mai3-broker/common/message"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/shopspring/decimal"
	"gopkg.in/guregu/null.v3"

	logger "github.com/sirupsen/logrus"
)

const TriggerPriceNotReach = "trigger price is not reached"
const PriceExceedsLimit = "price exceeds limit"

func (m *match) UpdateOrdersStatus(txID string, status model.TransactionStatus, transactionHash, blockHash string,
	blockNumber, blockTime uint64, successEvents []*model.TradeSuccessEvent, failedEvents []*model.TradeFailedEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ordersToNotify := make([]*model.Order, 0)
	err := m.dao.Transaction(m.ctx, false /* readonly */, func(dao dao.DAO) error {
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
		orders, err := m.updateOrdersByTradeEvent(dao, matchTx, successEvents, failedEvents)
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

func (m *match) updateOrdersByTradeEvent(dao dao.DAO, matchTx *model.MatchTransaction, successEvents []*model.TradeSuccessEvent, failedEvents []*model.TradeFailedEvent) ([]*model.Order, error) {
	ordersToNotify := make([]*model.Order, 0)
	// orders trade success
	orderSuccMap := make(map[string]decimal.Decimal)
	// orders trade failed, need to be canceled or rollbacked
	orderFailMap := make(map[string]decimal.Decimal)

	orderMatchMap := make(map[string]decimal.Decimal)
	orderHashes := make([]string, 0)

	if matchTx.Status == model.TransactionStatusSuccess {
		for _, event := range successEvents {
			logger.Infof("Trade Success: %+v", event)
			matchInfo := &model.MatchItem{
				OrderHash: event.OrderHash,
				Amount:    event.Amount,
			}
			matchTx.MatchResult.SuccItems = append(matchTx.MatchResult.SuccItems, matchInfo)
			orderSuccMap[event.OrderHash] = event.Amount
		}

		for _, event := range failedEvents {
			logger.Infof("Trade Failed: %+v", event)
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
				if err := m.rollbackOrderbook(oldAmount, matchAmount.Sub(amount), order); err != nil {
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
			// order not execute, reload order in orderbook
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

func (m *match) RollbackOrdersStatus(matchTx *model.MatchTransaction, status model.TransactionStatus, transactionHash, blockHash string,
	blockNumber, blockTime uint64, successEvents []*model.TradeSuccessEvent, failedEvents []*model.TradeFailedEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ordersToNotify := make([]*model.Order, 0)
	err := m.dao.Transaction(m.ctx, false /* readonly */, func(dao dao.DAO) error {
		// update match_transaction
		if matchTx.Status != status {
			matchTx.TransactionHash = null.StringFrom(transactionHash)
			matchTx.BlockHash = null.StringFrom(blockHash)
			matchTx.BlockNumber = null.IntFrom(int64(blockNumber))
			matchTx.ExecutedAt = null.TimeFrom(time.Unix(int64(blockTime), 0).UTC())
			matchTx.Status = status
		}

		// transaction fail to success/ success but event changed, rollback orders by event
		orders, err := m.rollbackOrdersByTradeEvent(dao, matchTx, successEvents, failedEvents)
		if err != nil {
			return err
		}
		ordersToNotify = append(ordersToNotify, orders...)
		if err := dao.UpdateMatchTransaction(matchTx); err != nil {
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

func (m *match) rollbackOrdersByTradeEvent(dao dao.DAO, matchTx *model.MatchTransaction, successEvents []*model.TradeSuccessEvent, failedEvents []*model.TradeFailedEvent) ([]*model.Order, error) {
	ordersToNotify := make([]*model.Order, 0)

	// success items before
	orderSuccItemsBefore := make(map[string]decimal.Decimal)
	// failed items before
	orderFailItemsBefore := make(map[string]decimal.Decimal)

	// orders trade success
	orderSuccMap := make(map[string]decimal.Decimal)
	// orders trade failed, need to be canceled
	orderFailMap := make(map[string]decimal.Decimal)

	orderMatchMap := make(map[string]decimal.Decimal)
	orderHashes := make([]string, 0)

	for _, item := range matchTx.MatchResult.SuccItems {
		orderSuccItemsBefore[item.OrderHash] = item.Amount
	}

	for _, item := range matchTx.MatchResult.FailedItems {
		orderFailItemsBefore[item.OrderHash] = item.Amount
	}

	matchTx.MatchResult.SuccItems = []*model.MatchItem{}
	matchTx.MatchResult.FailedItems = []*model.MatchItem{}

	for _, event := range successEvents {
		orderSuccMap[event.OrderHash] = event.Amount
	}

	for _, event := range failedEvents {
		// trigger price or price not match in contract, will rollback to orderbook
		if event.Reason == TriggerPriceNotReach || event.Reason == PriceExceedsLimit {
			continue
		}
		orderFailMap[event.OrderHash] = event.Amount
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
		// TODO consider checking amount
		if amount, ok := orderSuccItemsBefore[order.OrderHash]; ok {
			if _, ok := orderSuccMap[order.OrderHash]; ok {
				// order success
				item := &model.MatchItem{
					OrderHash: order.OrderHash,
					Amount:    amount,
				}
				matchTx.MatchResult.SuccItems = append(matchTx.MatchResult.SuccItems, item)
				continue
			}

			if _, ok := orderFailMap[order.OrderHash]; ok {
				// order success to fail
				order.ConfirmedAmount = order.ConfirmedAmount.Sub(amount)
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
				item := &model.MatchItem{
					OrderHash: order.OrderHash,
					Amount:    amount,
				}
				matchTx.MatchResult.FailedItems = append(matchTx.MatchResult.FailedItems, item)
				continue
			}

			// order success to orderbook
			oldAmount := order.AvailableAmount
			order.ConfirmedAmount = order.ConfirmedAmount.Sub(amount)
			order.AvailableAmount = order.AvailableAmount.Add(amount)
			if err := dao.UpdateOrder(order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}
			if err := m.rollbackOrderbook(oldAmount, amount, order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}
			ordersToNotify = append(ordersToNotify, order)
			continue
		}

		if amount, ok := orderFailItemsBefore[order.OrderHash]; ok {
			if _, ok := orderSuccMap[order.OrderHash]; ok {
				// order fail to success
				order.ConfirmedAmount = order.ConfirmedAmount.Add(amount)
				order.CanceledAmount = order.CanceledAmount.Sub(amount)
				// cancel reason
				reasons := make([]*model.OrderCancelReason, 0)
				reasons = append(reasons, order.CancelReasons...)
				order.CancelReasons = []*model.OrderCancelReason{}
				for _, reason := range reasons {
					if reason.TransactionHash == matchTx.TransactionHash.String {
						continue
					}
					order.CancelReasons = append(order.CancelReasons, reason)
				}
				if err := dao.UpdateOrder(order); err != nil {
					logger.Errorf("UpdateOrdersStatus:%s", err)
					return ordersToNotify, err
				}
				item := &model.MatchItem{
					OrderHash: order.OrderHash,
					Amount:    amount,
				}
				matchTx.MatchResult.SuccItems = append(matchTx.MatchResult.SuccItems, item)
				continue
			}

			if _, ok := orderFailMap[order.OrderHash]; ok {
				// order fail
				item := &model.MatchItem{
					OrderHash: order.OrderHash,
					Amount:    amount,
				}
				matchTx.MatchResult.FailedItems = append(matchTx.MatchResult.FailedItems, item)
				continue
			}

			// order fail to orderbook
			oldAmount := order.AvailableAmount
			order.CanceledAmount = order.CanceledAmount.Sub(amount)
			order.AvailableAmount = order.AvailableAmount.Add(amount)
			// cancel reason
			reasons := make([]*model.OrderCancelReason, 0)
			reasons = append(reasons, order.CancelReasons...)
			order.CancelReasons = []*model.OrderCancelReason{}
			for _, reason := range reasons {
				if reason.TransactionHash == matchTx.TransactionHash.String {
					continue
				}
				order.CancelReasons = append(order.CancelReasons, reason)
			}
			if err := dao.UpdateOrder(order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}
			if err := m.rollbackOrderbook(oldAmount, amount, order); err != nil {
				logger.Errorf("UpdateOrdersStatus:%s", err)
				return ordersToNotify, err
			}
			ordersToNotify = append(ordersToNotify, order)
			continue
		}

		if amount, ok := orderSuccMap[order.OrderHash]; ok {
			if order.AvailableAmount.Abs().GreaterThanOrEqual(amount.Abs()) {
				// orderbook to success
				oldAmount := order.AvailableAmount
				order.ConfirmedAmount = order.ConfirmedAmount.Add(amount)
				order.AvailableAmount = order.AvailableAmount.Sub(amount)
				if err := dao.UpdateOrder(order); err != nil {
					logger.Errorf("UpdateOrdersStatus:%s", err)
					return ordersToNotify, err
				}
				if err := m.rollbackOrderbook(oldAmount, amount.Neg(), order); err != nil {
					logger.Errorf("UpdateOrdersStatus:%s", err)
					return ordersToNotify, err
				}
				item := &model.MatchItem{
					OrderHash: order.OrderHash,
					Amount:    amount,
				}
				matchTx.MatchResult.SuccItems = append(matchTx.MatchResult.SuccItems, item)
				ordersToNotify = append(ordersToNotify, order)
			}
			continue
		}

		if amount, ok := orderFailMap[order.OrderHash]; ok {
			if order.AvailableAmount.Abs().GreaterThanOrEqual(amount.Abs()) {
				// orderbook to fail
				oldAmount := order.AvailableAmount
				order.CanceledAmount = order.CanceledAmount.Add(amount)
				order.AvailableAmount = order.AvailableAmount.Sub(amount)
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
				if err := m.rollbackOrderbook(oldAmount, amount.Neg(), order); err != nil {
					logger.Errorf("UpdateOrdersStatus:%s", err)
					return ordersToNotify, err
				}
				item := &model.MatchItem{
					OrderHash: order.OrderHash,
					Amount:    amount,
				}
				matchTx.MatchResult.FailedItems = append(matchTx.MatchResult.FailedItems, item)
				ordersToNotify = append(ordersToNotify, order)
			}
			continue
		}
	}
	return ordersToNotify, nil
}
