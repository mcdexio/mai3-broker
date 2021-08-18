package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mcdexio/mai3-broker/common/mai3"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

const ADDRESS_ZERO = "0x0000000000000000000000000000000000000000"

func (s *Server) GetOrders(p Param) (interface{}, error) {
	params := p.(*QueryOrderReq)
	var beforeOrderID, afterOrderID int64
	if params.BeforeOrderHash != "" {
		beforeOrder, err := s.dao.GetOrder(params.BeforeOrderHash)
		if err != nil {
			if dao.IsRecordNotFound(err) {
				return nil, OrderIDNotExistError(params.BeforeOrderHash)
			}
			return nil, InternalError(err)
		}

		beforeOrderID = beforeOrder.ID
	}

	if params.AfterOrderHash != "" {
		afterOrder, err := s.dao.GetOrder(params.AfterOrderHash)
		if err != nil {
			if dao.IsRecordNotFound(err) {
				return nil, OrderIDNotExistError(params.AfterOrderHash)
			}
			return nil, InternalError(err)
		}
		afterOrderID = afterOrder.ID
	}

	if params.LiquidityPoolAddress != "" {
		_, err := s.dao.GetPerpetualByPoolAddressAndIndex(params.LiquidityPoolAddress, params.PerpetualIndex, true)
		if err != nil {
			if dao.IsRecordNotFound(err) {
				return nil, PerpetualNotFoundError(params.LiquidityPoolAddress, params.PerpetualIndex)
			}
			return nil, InternalError(err)
		}
	}

	queryStatus := make([]model.OrderStatus, 0)
	if params.Status != "all" {
		filterStatus := strings.Split(params.Status, model.QueryParamSeperator)
		for _, status := range filterStatus {
			queryStatus = append(queryStatus, model.OrderStatus(status))
		}
	}

	limit := 20
	if params.Limit > 0 {
		limit = params.Limit
	}
	var orders []*model.Order
	var err error
	if params.BeginTime != 0 || params.EndTime != 0 {
		beginTime := time.Unix(params.BeginTime, 0).UTC()
		endTime := time.Unix(params.EndTime, 0).UTC()
		if params.BeginTime == 0 {
			beginTime = time.Time{}
		}
		if params.EndTime == 0 {
			endTime = time.Time{}
		}
		orders, err = s.dao.QueryOrderWithCreateTime(params.Address, params.LiquidityPoolAddress, params.PerpetualIndex, queryStatus, beforeOrderID, afterOrderID, beginTime, endTime, limit)
		if err != nil {
			return nil, InternalError(err)
		}
	} else {
		orders, err = s.dao.QueryOrder(params.Address, params.LiquidityPoolAddress, params.PerpetualIndex, queryStatus, beforeOrderID, afterOrderID, limit)
		if err != nil {
			return nil, InternalError(err)
		}
	}

	res := &QueryOrdersResp{
		Orders: make([]*model.Order, 0),
	}
	res.Orders = append(res.Orders, orders...)
	return res, nil
}

func (s *Server) GetOrderByOrderHash(p Param) (interface{}, error) {
	params := p.(*QuerySingleOrderReq)
	order, err := s.dao.GetOrder(params.OrderHash)
	if err != nil {
		if dao.IsRecordNotFound(err) {
			return nil, OrderIDNotExistError(params.OrderHash)
		}
		return nil, InternalError(err)
	}
	res := &QuerySingleOrderResp{
		Order: order,
	}
	return res, nil
}

func (s *Server) GetOrdersByOrderHashs(p Param) (interface{}, error) {
	params := p.(*QueryOrdersByOrderHashsReq)
	orders, err := s.dao.GetOrderByHashs(params.OrderHashs)

	if err != nil {
		return nil, InternalError(err)
	}

	res := &QueryOrdersResp{
		Orders: make([]*model.Order, 0),
	}
	res.Orders = append(res.Orders, orders...)
	return res, nil
}

func GetOrderFromPalceOrderReq(params *PlaceOrderReq) (*model.Order, error) {
	err := validatePlaceOrder(params)
	if err != nil {
		return nil, err
	}

	order := model.Order{}
	order.OrderHash = params.OrderHash
	order.OrderParam.TraderAddress = strings.ToLower(params.Address)
	order.OrderParam.Type = model.OrderType(params.OrderType)
	order.Status = model.OrderPending
	amount, _ := decimal.NewFromString(params.Amount)
	order.OrderParam.Amount = amount.Round(mai3.DECIMALS)
	order.OrderParam.ChainID = params.ChainID
	price, _ := decimal.NewFromString(params.Price)
	order.OrderParam.Price = price.Round(mai3.DECIMALS)
	triggerPrice, _ := decimal.NewFromString(params.TriggerPrice)
	order.OrderParam.TriggerPrice = triggerPrice.Round(mai3.DECIMALS)
	order.OrderParam.IsCloseOnly = params.IsCloseOnly
	order.OrderParam.ExpiresAt = time.Unix(params.ExpiresAt, 0).UTC()
	order.OrderParam.Salt = params.Salt
	sig := model.OrderSignature{
		R:        params.R,
		S:        params.S,
		V:        params.V,
		SignType: params.SignType,
	}
	sigJSON, err := json.Marshal(sig)
	if err != nil {
		return nil, InternalError(fmt.Errorf("marshalSignature:%w", err))
	}
	order.OrderParam.Signature = string(sigJSON)
	minTradeAmount, _ := decimal.NewFromString(params.MinTradeAmount)
	order.OrderParam.MinTradeAmount = minTradeAmount.Round(mai3.DECIMALS)
	order.OrderParam.BrokerFeeLimit = params.BrokerFeeLimit
	order.OrderParam.LiquidityPoolAddress = strings.ToLower(params.LiquidityPoolAddress)
	order.OrderParam.PerpetualIndex = params.PerpetualIndex
	order.OrderParam.BrokerAddress = strings.ToLower(params.BrokerAddress)
	if !strings.EqualFold(conf.Conf.BrokerAddress, order.OrderParam.BrokerAddress) {
		return nil, InternalError(fmt.Errorf("unSupport brokerAddress:%s", params.BrokerAddress))
	}
	order.OrderParam.RelayerAddress = strings.ToLower(params.RelayerAddress)
	order.OrderParam.ReferrerAddress = params.ReferrerAddress
	if params.ReferrerAddress == "" {
		order.OrderParam.ReferrerAddress = ADDRESS_ZERO
	}
	order.AvailableAmount = order.OrderParam.Amount
	order.ConfirmedAmount = decimal.Zero
	order.CanceledAmount = decimal.Zero
	order.PendingAmount = decimal.Zero
	now := time.Now().UTC()
	order.CreatedAt = now
	order.UpdatedAt = now

	// check orderhash
	flags := mai3.GenerateOrderFlags(order.Type, order.IsCloseOnly)
	orderHash, err := mai3.GetOrderHash(order.TraderAddress, order.BrokerAddress, order.RelayerAddress, order.ReferrerAddress, order.LiquidityPoolAddress,
		order.MinTradeAmount, order.Amount, order.Price, order.TriggerPrice, order.ChainID, params.ExpiresAt, order.PerpetualIndex, order.BrokerFeeLimit,
		int64(flags), order.Salt)
	if err != nil {
		return nil, InternalError(fmt.Errorf("get order hash fail err:%s", err))
	}

	if utils.Bytes2HexP(orderHash) != params.OrderHash {
		return nil, InternalError(fmt.Errorf("order hash not match"))
	}

	// check signature
	signature := "0x" + params.R + params.S + params.V
	valid, err := mai3.IsValidSignature(params.Address, params.OrderHash, signature, params.SignType)
	if err != nil {
		return nil, InternalError(fmt.Errorf("check signature fail"))
	}
	if !valid {
		return nil, BadSignatureError()
	}
	return &order, nil
}

func (s *Server) PlaceOrder(p Param) (interface{}, error) {
	params := p.(*PlaceOrderReq)
	logger.Infof("New order : %s", params.OrderHash)

	order, err := GetOrderFromPalceOrderReq(params)
	if err != nil {
		return nil, err
	}

	perpetual, err := s.dao.GetPerpetualByPoolAddressAndIndex(order.LiquidityPoolAddress, order.PerpetualIndex, true)
	if err != nil {
		if dao.IsRecordNotFound(err) {
			return nil, PerpetualNotFoundError(order.LiquidityPoolAddress, order.PerpetualIndex)
		}
		return nil, InternalError(err)
	}

	// set collateral address
	order.CollateralAddress = perpetual.CollateralAddress

	// check ChainID
	if conf.Conf.ChainID != params.ChainID {
		return nil, ChainIDError(params.ChainID)
	}

	_, err = s.dao.GetOrder(params.OrderHash)
	if err == nil {
		return nil, OrderHashExistError(params.OrderHash)
	} else if !dao.IsRecordNotFound(err) {
		return nil, InternalError(errors.New("get order fail"))
	}

	errID := s.match.NewOrder(order)
	switch errID {
	case model.MatchOK:
		// jwt, err := auth.SignJwt(order.TraderAddress)
		// if err != nil {
		// 	logger.Warnf("fail to sign jwt for '%s' when place order:%s", order.TraderAddress, err.Error())
		// 	return &PlaceOrderResp{}, nil
		// }
		// expires := auth.JwtExpiration / time.Second * 1000
		return &PlaceOrderResp{Order: order}, nil
	case model.MatchInternalErrorID:
		return nil, InternalError(errors.New("match new order error"))
	case model.MatchMaxOrderNumReachID:
		return nil, MaxOrderNumReachError()
	case model.MatchInsufficientBalanceErrorID:
		return nil, InsufficientBalanceError()
	case model.MatchGasNotEnoughErrorID:
		return nil, GasBalanceError()
	case model.MatchCloseOnlyErrorID:
		return nil, CloseOnlyError()
	default:
		return nil, InternalError(errors.New("unknown match error"))
	}
}

func validatePlaceOrder(req *PlaceOrderReq) error {
	// Amount
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return InvalidPriceAmountError(fmt.Sprintf("parse amount[%s] error", req.Amount))
	}
	if amount.Equal(decimal.Zero) {
		return InvalidPriceAmountError("amount = 0")
	}

	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		return InvalidPriceAmountError(fmt.Sprintf("parse price[%s] error", req.Price))
	}

	if price.LessThanOrEqual(decimal.Zero) {
		return InvalidPriceAmountError("price <= 0")
	}

	minTradeAmount, err := decimal.NewFromString(req.MinTradeAmount)
	if err != nil {
		return InvalidPriceAmountError(fmt.Sprintf("parse minTradeAmount[%s] error", req.MinTradeAmount))
	}

	if minTradeAmount.IsZero() || minTradeAmount.Abs().GreaterThan(amount.Abs()) {
		return InvalidPriceAmountError("minTradeAmount invalid")
	}

	// order deadline
	expiresAt := time.Unix(req.ExpiresAt, 0).UTC()
	now := time.Now().UTC()
	if now.After(expiresAt) {
		return OrderExpired()
	}

	// trigger Price
	switch model.OrderType(req.OrderType) {
	case model.StopLimitOrder, model.TakeProfitOrder:
		triggerPrice, err := decimal.NewFromString(req.TriggerPrice)
		if err != nil {
			return InvalidPriceAmountError(fmt.Sprintf("parse price[%s] error", req.TriggerPrice))
		}

		if triggerPrice.LessThanOrEqual(decimal.Zero) {
			return InvalidPriceAmountError("trigger price <= 0")
		}
		if (amount.IsPositive() && triggerPrice.GreaterThan(price)) ||
			(amount.IsNegative() && triggerPrice.LessThan(price)) {
			return InvalidPriceAmountError("trigger price and limit price are not appropriate")
		}
	case model.LimitOrder:
	default:
		return InvalidOrderTypeError()
	}

	// broker contract address
	if !strings.EqualFold(req.BrokerAddress, conf.Conf.BrokerAddress) {
		return BrokerAddressError(req.BrokerAddress)
	}

	return nil
}

func (s *Server) CancelOrder(p Param) (interface{}, error) {
	params := p.(*CancelOrderReq)
	order, err := s.dao.GetOrder(params.OrderHash)
	if err != nil {
		if dao.IsRecordNotFound(err) {
			return nil, OrderIDNotExistError(params.OrderHash)
		}
		return nil, InternalError(err)
	}

	if !strings.EqualFold(params.Address, order.TraderAddress) {
		return nil, OrderAuthError(params.OrderHash)
	}

	if err = s.match.CancelOrder(order.LiquidityPoolAddress, order.PerpetualIndex, order.OrderHash); err != nil {
		return nil, InternalError(err)
	}
	return nil, nil
}

func (s *Server) CancelAllOrders(p Param) (interface{}, error) {
	params := p.(*CancelAllOrdersReq)
	orders, err := s.dao.QueryOrder(strings.ToLower(params.Address), "", 0, []model.OrderStatus{model.OrderPending}, 0, 0, 0)
	if err != nil {
		return nil, InternalError(err)
	}
	if len(orders) == 0 {
		return nil, nil
	}
	go func() {
		for _, order := range orders {
			if err = s.match.CancelOrder(order.LiquidityPoolAddress, order.PerpetualIndex, order.OrderHash); err != nil {
				logger.Errorf("CancelAllOrders. order_hash:%s, err:%s", order.OrderHash, err.Error())
			}
		}
	}()
	return nil, nil
}
