package match

import (
	"github.com/mcdexio/mai3-broker/common/message"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

func (m *match) CancelOrder(orderHash string, reason model.CancelReasonType, cancelAll bool, cancelAmount decimal.Decimal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancelOrderWithoutLock(orderHash, reason, cancelAll, cancelAmount)
}

func (m *match) cancelOrderWithoutLock(orderHash string, reason model.CancelReasonType, cancelAll bool, cancelAmount decimal.Decimal) error {
	var order *model.Order
	cancelBookAmount := cancelAmount
	cancelDBAmount := cancelAmount
	err := m.dao.Transaction(m.ctx, false /* readonly */, func(dao dao.DAO) error {
		var err error
		order, err = dao.GetOrder(orderHash)
		if err != nil {
			return err
		}
		orderbook := m.orderbook
		bookOrder, ok := orderbook.GetOrder(orderHash, order.Amount.IsNegative(), order.Price)
		if !ok {
			if order.AvailableAmount.IsZero() {
				logger.Warnf("cancel order:order[%s] is closed.", orderHash)
				order = nil
				return nil
			} else {
				logger.Errorf("cancel order: order[%s] not exists in book, cancel all in db!", orderHash)
				cancelDBAmount = order.AvailableAmount
				cancelBookAmount = decimal.Zero
			}
		} else {
			if !order.AvailableAmount.Equal(bookOrder.Amount) {
				logger.Errorf("cancel order: order[%s] amount mismatch between db[%s] and book[%s] cancel all!", orderHash, order.AvailableAmount, bookOrder.Amount)
				cancelAll = true
			}
			if cancelAll {
				cancelDBAmount = order.AvailableAmount
				cancelBookAmount = bookOrder.Amount
			}
			if order.AvailableAmount.Abs().LessThan(cancelAmount.Abs()) {
				logger.Warnf("cancel amount[%s] larger than db available amount[%s]", cancelAmount, order.AvailableAmount)
				cancelDBAmount = order.AvailableAmount
			}
			if bookOrder.Amount.Abs().LessThan(cancelAmount.Abs()) {
				logger.Warnf("cancel amount[%s] larger than book available amount[%s]", cancelAmount, bookOrder.Amount)
				cancelBookAmount = order.AvailableAmount
			}
		}

		if !cancelBookAmount.IsZero() {
			if err := orderbook.ChangeOrder(bookOrder, cancelBookAmount.Neg()); err != nil {
				return err
			}
		}

		if !cancelDBAmount.IsZero() {
			if err = model.CancelOrder(order, reason, cancelDBAmount); err != nil {
				return err
			}

			if err = dao.UpdateOrder(order); err != nil {
				return err
			}
		}

		m.deleteOrderTimer(orderHash)
		return nil
	})
	if err == nil && order != nil && !cancelDBAmount.IsZero() {
		// notice websocket for cancel order
		wsMsg := message.WebSocketMessage{
			ChannelID: message.GetAccountChannelID(order.TraderAddress),
			Payload: message.WebSocketOrderChangePayload{
				Type:  message.WsTypeOrderChange,
				Order: order,
			},
		}
		m.wsChan <- wsMsg
	}
	return err
}
