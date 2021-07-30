package model

import (
	"time"

	"github.com/shopspring/decimal"
)

const QueryParamSeperator = ","

type CancelReasonType string

const (
	CancelReasonExpired           CancelReasonType = "EXPIRED"
	CancelReasonCloseOnly         CancelReasonType = "CANCELED_BY_CLOSE_ONLY"
	CancelReasonUserCancel        CancelReasonType = "CANCELED_BY_USER"
	CancelReasonTransactionFail   CancelReasonType = "TRANSACTION_FAIL"
	CancelReasonRemainTooSmall    CancelReasonType = "REMAIN_TOO_SMALL"
	CancelReasonInsufficientFunds CancelReasonType = "INSUFFICIENT_FUNDS"
	CancelReasonGasNotEnough      CancelReasonType = "GAS_NOT_ENOUGH"
	CancelReasonContractSettled   CancelReasonType = "CONTRACT_SETTLED"
)

const (
	MatchOK                         = "match.ok"
	MatchInternalErrorID            = "match.error.InternalError"
	MatchGasNotEnoughErrorID        = "match.error.GasNotEnough"
	MatchCloseOnlyErrorID           = "match.error.CloseOnly"
	MatchInsufficientBalanceErrorID = "match.error.InsufficientBalance"
	MatchMaxOrderNumReachID         = "match.error.MaxOrderNumReach"
)

type OrderType int

const (
	MASK_CLOSE_ONLY                    = 0x80000000
	MASK_MARKET_ORDER                  = 0x40000000
	MASK_STOP_LOSS_ORDER               = 0x20000000
	MASK_TAKE_PROFIT_ORDER             = 0x10000000
	MASK_USE_TARGET_LEVERAGE           = 0x08000000
	LimitOrder               OrderType = 1
	StopLimitOrder           OrderType = 2
	TakeProfitOrder          OrderType = 3
)

type OrderStatus string

const (
	OrderCanceled      OrderStatus = "canceled"
	OrderPending       OrderStatus = "pending"
	OrderPartialFilled OrderStatus = "partial_filled"
	OrderFullFilled    OrderStatus = "full_filled"
)

type OrderSignature struct {
	R        string `json:"r"`
	S        string `json:"s"`
	V        string `json:"v"`
	SignType string `json:"signType"`
}

type OrderParam struct {
	TraderAddress        string          `json:"traderAddress" db:"trader_address"`
	LiquidityPoolAddress string          `json:"liquidityPoolAddress" db:"liquidity_pool_address"`
	PerpetualIndex       int64           `json:"perpetualIndex" db:"perpetual_index"`
	RelayerAddress       string          `json:"relayerAddress" db:"relayer_address"`
	BrokerAddress        string          `json:"brokerAddress" db:"broker_address"`
	ReferrerAddress      string          `json:"referrerAddress" db:"referrer_address"`
	Type                 OrderType       `json:"type" db:"type"`
	Price                decimal.Decimal `json:"price" db:"price"`
	Amount               decimal.Decimal `json:"amount" db:"amount"`
	MinTradeAmount       decimal.Decimal `json:"minTradeAmount" db:"min_trade_amount"`
	TriggerPrice         decimal.Decimal `json:"triggerPrice" db:"trigger_price"`
	BrokerFeeLimit       int64           `json:"brokerFeeLimit" db:"broker_fee_limit"`
	ExpiresAt            time.Time       `json:"expiresAt" db:"expires_at"`
	Salt                 int64           `json:"-" db:"salt"`
	IsCloseOnly          bool            `json:"isCloseOnly" db:"is_close_only"`
	ChainID              int64           `json:"-" db:"chain_id"`
	Signature            string          `json:"-" db:"signature"`
}

type Order struct {
	OrderParam
	ID                int64                `json:"-" db:"id" primaryKey:"true" gorm:"primary_key"`
	OrderHash         string               `json:"orderHash" db:"order_hash"`
	OldStatus         OrderStatus          `json:"oldStatus" sql:"-"`
	Status            OrderStatus          `json:"status" db:"status"`
	CollateralAddress string               `json:"collateralAddress" db:"collateral_address"`
	AvailableAmount   decimal.Decimal      `json:"availableAmount" db:"available_amount"`
	ConfirmedAmount   decimal.Decimal      `json:"confirmedAmount" db:"confirmed_amount"`
	CanceledAmount    decimal.Decimal      `json:"canceledAmount" db:"canceled_amount"`
	PendingAmount     decimal.Decimal      `json:"pendingAmount" db:"pending_amount"`
	GasFeeLimit       int64                `json:"gasFeeLimit" db:"gas_fee_limit"`
	CreatedAt         time.Time            `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time            `json:"updatedAt" db:"updated_at"`
	CheckAmount       decimal.Decimal      `json:"-" sql:"-"`
	CancelReasons     []*OrderCancelReason `json:"cancelReasons" sql:"-"`
}

//OrderCancelReason records of cancel
type OrderCancelReason struct {
	Reason          CancelReasonType `json:"reason"`
	Amount          decimal.Decimal  `json:"amount"`
	CanceledAt      time.Time        `json:"canceledAt"`
	TransactionHash string           `json:"transactionHash,omitempty"`
}

type OrderCancel struct {
	OrderHash string
	Status    OrderStatus
	ToCancel  decimal.Decimal
}
