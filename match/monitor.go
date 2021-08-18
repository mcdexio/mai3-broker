package match

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

type OrderCancel struct {
	LiquidityPoolAddress string
	PerpetualIndex       int64
	OrderHash            string
	Status               model.OrderStatus
	ToCancel             decimal.Decimal
	Reason               model.CancelReasonType
}

func (s *Server) checkActiveOrders(ctx context.Context) error {
	logger.Infof("match monitor start")
	for {
		select {
		case <-ctx.Done():
			logger.Infof("match monitor end")
			return nil
		case <-time.After(conf.Conf.MatchMonitorInterval):
			s.checkPerpUserOrders()
		}
	}
}

func (s *Server) checkPerpUserOrders() {
	users, err := s.dao.GetPendingOrderUsers("", 0, []model.OrderStatus{model.OrderPending})
	if err != nil {
		logger.Errorf("monitor: GetPendingOrderUsers %s", err)
		return
	}
	if len(users) == 0 {
		return
	}

	for _, user := range users {
		cancels := s.checkUserPendingOrders(user)
		for _, cancel := range cancels {
			handler := s.getMatchHandler(cancel.LiquidityPoolAddress, cancel.PerpetualIndex)
			if handler != nil {
				err := handler.CancelOrder(cancel.OrderHash, cancel.Reason, false, cancel.ToCancel)
				if err != nil {
					logger.Errorf("cancel Order fail! err:%s", err)
				}
			}
		}
	}
}

type OrdersCollateral struct {
	WalletBalance decimal.Decimal
	OrderMap      map[string]*OrdersPerpMap
}

type OrdersPerpMap struct {
	LiquidityPoolAddress string
	PerpetualIndex       int64
	Account              *model.AccountStorage
	PoolStorage          *model.LiquidityPoolStorage
	Orders               []*model.Order
}

func (s *Server) singleOrderCheck(order *model.Order, poolStorage *model.LiquidityPoolStorage, account *model.AccountStorage) *OrderCancel {
	// because broker address was signed, if it was changed, orders need to be canceled
	if order.OrderParam.BrokerAddress != strings.ToLower(conf.Conf.BrokerAddress) {
		return &OrderCancel{
			LiquidityPoolAddress: order.LiquidityPoolAddress,
			PerpetualIndex:       order.PerpetualIndex,
			OrderHash:            order.OrderHash,
			Status:               order.Status,
			ToCancel:             order.AvailableAmount,
			Reason:               model.CancelReasonTransactionFail,
		}
	}

	// cancel order if perpetual status is not normal
	perpetual, ok := poolStorage.Perpetuals[order.PerpetualIndex]
	if !ok || !perpetual.IsNormal {
		return &OrderCancel{
			LiquidityPoolAddress: order.LiquidityPoolAddress,
			PerpetualIndex:       order.PerpetualIndex,
			OrderHash:            order.OrderHash,
			Status:               order.Status,
			ToCancel:             order.AvailableAmount,
			Reason:               model.CancelReasonContractSettled,
		}
	}

	// close only check
	cancelAmount := CheckCloseOnly(account, order)
	if !cancelAmount.IsZero() {
		return &OrderCancel{
			LiquidityPoolAddress: order.LiquidityPoolAddress,
			PerpetualIndex:       order.PerpetualIndex,
			OrderHash:            order.OrderHash,
			Status:               order.Status,
			ToCancel:             cancelAmount,
			Reason:               model.CancelReasonCloseOnly,
		}
	}
	return nil
}

func (s *Server) splitActiveOrdersInEachPerpetual(orders []*model.Order) (map[string]*OrdersCollateral, []*OrderCancel, error) {
	res := make(map[string]*OrdersCollateral)
	cancels := make([]*OrderCancel, 0)
	for _, order := range orders {
		// orders split in different collateral
		collateralMap, ok := res[order.CollateralAddress]
		if ok {
			// orders split in different perpetual
			perpetualID := fmt.Sprintf("%s-%d", order.LiquidityPoolAddress, order.PerpetualIndex)
			ordersPerp, ok := collateralMap.OrderMap[perpetualID]
			if ok {
				// check order
				cancel := s.singleOrderCheck(order, ordersPerp.PoolStorage, ordersPerp.Account)
				if cancel != nil {
					cancels = append(cancels, cancel)
					continue
				}
				// pending amount set to zero for monitor
				order.PendingAmount = decimal.Zero
				ordersPerp.Orders = append(ordersPerp.Orders, order)
			} else {
				poolStorage := s.poolSyncer.GetPoolStorage(order.LiquidityPoolAddress)
				if poolStorage == nil {
					return res, cancels, fmt.Errorf("get pool storage error")
				}

				account, err := s.chainCli.GetAccountStorage(s.ctx, conf.Conf.ReaderAddress, order.PerpetualIndex, order.LiquidityPoolAddress, order.TraderAddress)
				if account == nil || err != nil {
					return res, cancels, err
				}

				// check order
				cancel := s.singleOrderCheck(order, poolStorage, account)
				if cancel != nil {
					cancels = append(cancels, cancel)
					continue
				}

				collateralMap.OrderMap[perpetualID] = &OrdersPerpMap{
					LiquidityPoolAddress: order.LiquidityPoolAddress,
					PerpetualIndex:       order.PerpetualIndex,
					Account:              account,
					PoolStorage:          poolStorage,
					Orders:               []*model.Order{order},
				}
			}
		} else {
			// orders split in different perpetual
			perpetualID := fmt.Sprintf("%s-%d", order.LiquidityPoolAddress, order.PerpetualIndex)
			poolStorage := s.poolSyncer.GetPoolStorage(order.LiquidityPoolAddress)
			if poolStorage == nil {
				return res, cancels, fmt.Errorf("get pool storage error")
			}

			account, err := s.chainCli.GetAccountStorage(s.ctx, conf.Conf.ReaderAddress, order.PerpetualIndex, order.LiquidityPoolAddress, order.TraderAddress)
			if account == nil || err != nil {
				return res, cancels, err
			}
			// check order
			cancel := s.singleOrderCheck(order, poolStorage, account)
			if cancel != nil {
				cancels = append(cancels, cancel)
				continue
			}

			handler := s.getMatchHandler(order.LiquidityPoolAddress, order.PerpetualIndex)
			if handler == nil {
				return res, cancels, fmt.Errorf("perp not start. perpetualID: %s-%d", order.LiquidityPoolAddress, order.PerpetualIndex)
			}
			collateralDecimals := handler.GetCollateralDecimal()

			// get collateral wallet balance
			balance, err := s.chainCli.BalanceOf(s.ctx, order.CollateralAddress, order.TraderAddress, collateralDecimals)
			if err != nil {
				return res, cancels, err
			}
			allowance, err := s.chainCli.Allowance(s.ctx, order.CollateralAddress, order.TraderAddress, order.LiquidityPoolAddress, collateralDecimals)
			if err != nil {
				return res, cancels, err
			}

			walletBalance := decimal.Min(balance, allowance)
			res[order.CollateralAddress] = &OrdersCollateral{
				WalletBalance: walletBalance,
				OrderMap:      make(map[string]*OrdersPerpMap),
			}

			// pending amount set to zero for monitor
			order.PendingAmount = decimal.Zero
			res[order.CollateralAddress].OrderMap[perpetualID] = &OrdersPerpMap{
				LiquidityPoolAddress: order.LiquidityPoolAddress,
				PerpetualIndex:       order.PerpetualIndex,
				Account:              account,
				PoolStorage:          poolStorage,
				Orders:               []*model.Order{order},
			}
		}
	}

	return res, cancels, nil
}

func (s *Server) checkUserPendingOrders(user string) []*OrderCancel {
	cancels := make([]*OrderCancel, 0)

	// check order margin and close Only order
	orders, err := s.dao.QueryOrder(user, "", 0, []model.OrderStatus{model.OrderPending}, 0, 0, 0)
	if err != nil {
		logger.Errorf("checkUserPendingOrders err:%v", err)
		return cancels
	}

	orderMap, orderCancels, err := s.splitActiveOrdersInEachPerpetual(orders)
	if err != nil {
		logger.Errorf("splitActiveOrdersInEachPerpetual err:%v", err)
		return cancels
	}

	cancels = append(cancels, orderCancels...)
	// check remain orders available margin
	for _, collateralMap := range orderMap {
		walletBalance := collateralMap.WalletBalance
		for _, v := range collateralMap.OrderMap {
			v.Account.WalletBalance = walletBalance
			cancelsInsufficientFunds, available := ComputeOrderAvailable(v.PoolStorage, v.PerpetualIndex, v.Account, v.Orders)
			cancels = append(cancels, cancelsInsufficientFunds...)
			walletBalance = available
		}
	}
	return cancels
}
