package model

import (
	"fmt"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v3"
	"time"
)

func addCancelReason(order *Order, reason CancelReasonType, amount decimal.Decimal, tradeID string, transactionHash string, executedAt null.Time) {
	r := &OrderCancelReason{
		Reason:          reason,
		Amount:          amount,
		TransactionHash: transactionHash,
	}
	if executedAt.Valid {
		r.CanceledAt = executedAt.Time
	} else {
		r.CanceledAt = time.Now().UTC()
	}
	order.CancelReasons = append(order.CancelReasons, r)
}

func CancelOrder(order *Order, reason CancelReasonType, amount decimal.Decimal) error {
	if reason == "" {
		return fmt.Errorf("cancel order, empty reason")
	}
	if amount.IsZero() || !utils.HasTheSameSign(amount, order.Amount) {
		return fmt.Errorf("cancel order fail, bad amount[%s]", amount)
	}
	if order.AvailableAmount.Abs().LessThan(amount.Abs()) {
		return fmt.Errorf("cancel order fail, cancel amount[%s] larger than available[%s]", amount, order.AvailableAmount)
	}
	order.AvailableAmount = order.AvailableAmount.Sub(amount)
	order.CanceledAmount = order.CanceledAmount.Add(amount)

	if reason != CancelReasonUserCancel {
		logger.Warnf("cancel order[%s] reason[%s] amount[%s], after: available[%s] pending[%s] canceled[%s]",
			order.OrderHash, reason, amount, order.AvailableAmount, order.PendingAmount, order.CanceledAmount)
	}

	addCancelReason(order, reason, amount, "", "", null.Time{})

	return nil
}
