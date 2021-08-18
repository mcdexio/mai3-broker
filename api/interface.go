package api

import (
	"github.com/mcdexio/mai3-broker/common/model"
)

type (
	// BaseReq represents a basic request data containing wallet addresses
	// from which the request is sent
	BaseReq struct {
		Address string `json:"address" query:"address"`
	}

	// BaseResp represents a basic response data containing status and description to indicates
	// the executing result of an api request
	BaseResp struct {
		Status int    `json:"status"`
		Desc   string `json:"desc"`
	}

	PlaceOrderReq struct {
		BaseReq
		OrderHash            string `json:"orderHash"  validate:"required"`
		LiquidityPoolAddress string `json:"liquidityPoolAddress" validate:"required"`
		PerpetualIndex       int64  `json:"perpetualIndex"`
		BrokerAddress        string `json:"brokerAddress" validate:"required"`
		RelayerAddress       string `json:"relayerAddress" validate:"required"`
		ReferrerAddress      string `json:"referrerAddress"`
		OrderType            int    `json:"orderType" validate:"required,oneof=0 1 2"`
		Price                string `json:"price"     validate:"required"`
		TriggerPrice         string `json:"triggerPrice"`
		Amount               string `json:"amount"    validate:"required"`
		MinTradeAmount       string `json:"minTradeAmount" validate:"required"`
		BrokerFeeLimit       int64  `json:"brokerFeeLimit"`
		Salt                 int64  `json:"salt" validate:"required"`
		ExpiresAt            int64  `json:"expiresAt" validate:"required"`
		R                    string `json:"r" validate:"required"`
		S                    string `json:"s" validate:"required"`
		V                    string `json:"v" validate:"required"`
		SignType             string `json:"signType" validate:"required"`
		IsCloseOnly          bool   `json:"isCloseOnly"`
		ChainID              int64  `json:"chainID" validate:"required"`
	}

	PlaceOrderResp struct {
		Order *model.Order `json:"order"`
	}

	CancelOrderReq struct {
		BaseReq
		OrderHash string `json:"orderHash" param:"orderHash" validate:"required,len=66"`
	}

	CancelAllOrdersReq struct {
		BaseReq
	}

	QuerySingleOrderReq struct {
		BaseReq
		OrderHash string `json:"orderHash" param:"orderHash" validate:"required"`
	}

	QuerySingleOrderResp struct {
		Order *model.Order `json:"order"`
	}

	QueryOrdersByOrderHashsReq struct {
		BaseReq
		OrderHashs []string `json:"orderIDs" validate:"required"`
	}

	QueryOrderReq struct {
		BaseReq
		LiquidityPoolAddress string `json:"liquidityPoolAddress" query:"liquidityPoolAddress"`
		PerpetualIndex       int64  `json:"perpetualIndex" query:"perpetualIndex"`
		Status               string `json:"status" query:"status"`
		BeforeOrderHash      string `json:"beforeOrderHash" query:"beforeOrderHash"`
		AfterOrderHash       string `json:"afterOrderHash"  query:"afterOrderHash"`
		BeginTime            int64  `json:"beginTime" query:"beginTime"`
		EndTime              int64  `json:"endTime" query:"endTime"`
		Limit                int    `json:"limit" query:"limit"`
	}

	QueryOrdersResp struct {
		Orders []*model.Order `json:"orders"`
	}

	GetPerpetualReq struct {
		BaseReq
		LiquidityPoolAddress string `json:"liquidityPoolAddress" validate:"required"`
		PerpetualIndex       int64  `json:"perpetualIndex"`
	}

	GetPerpetualResp struct {
		Perpetual *model.Perpetual `json:"perpetual"`
	}

	GetBrokerRelayReq struct {
		BaseReq
		LiquidityPoolAddress string `json:"liquidityPoolAddress" validate:"required"`
		PerpetualIndex       int64  `json:"perpetualIndex"`
	}

	GetBrokerRelayResp struct {
		BrokerAddress  string `json:"brokerAddress"`
		Version        int    `json:"version"`
		RelayerAddress string `json:"relayerAddress"`
	}
)

func (b *BaseReq) GetAddress() string {
	return b.Address
}

func (b *BaseReq) SetAddress(address string) {
	b.Address = address
}

type Param interface {
	GetAddress() string
	SetAddress(address string)
}
