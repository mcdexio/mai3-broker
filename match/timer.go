package match

import (
	"time"

	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

func (m *match) onOrderExpired(orderID string) {
	m.deleteOrderTimer(orderID)
	if err := m.CancelOrder(orderID, model.CancelReasonExpired, true, decimal.Zero); err != nil {
		logger.Errorf("Cancel Order error perpetual:%s-%d, orderHash:%s", m.perpetual.LiquidityPoolAddress, m.perpetual.PerpetualIndex, orderID)
	}
}

func (m *match) setExpirationTimer(orderID string, expiresAt time.Time) error {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()
	now := time.Now().UTC()
	if !expiresAt.After(now) {
		go m.onOrderExpired(orderID)
		return nil
	}
	t := time.AfterFunc(expiresAt.Sub(now), func() { m.onOrderExpired(orderID) })
	if t != nil {
		m.timers[orderID] = t
	}

	return nil
}

func (m *match) stopTimers() {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()
	for k, t := range m.timers {
		t.Stop()
		delete(m.timers, k)
	}
}

func (m *match) deleteOrderTimer(orderID string) bool {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()
	if t, ok := m.timers[orderID]; ok {
		t.Stop()
		delete(m.timers, orderID)
		return true
	}
	return false
}
