package model

import (
	"github.com/shopspring/decimal"
	"gopkg.in/guregu/null.v3"
	"time"
)

type TransactionStatus string

const (
	//TransactionStatusInit Transaction is initialized
	TransactionStatusInit TransactionStatus = "INIT"

	//TransactionStatusPending Transaction is signed by wallet, the transaction hash is created
	TransactionStatusPending TransactionStatus = "PENDING"

	//TransactionStatusSuccess Transaction succeeded on block chain
	TransactionStatusSuccess TransactionStatus = "SUCCESS"

	//TransactionStatusFail Transaction failed on block chain
	TransactionStatusFail TransactionStatus = "FAIL"

	//TransactionStatusAbort Transaction is abort due to system error
	TransactionStatusAbort TransactionStatus = "ABORT"
)

type MatchResult struct {
	MatchItems  []*MatchItem `json:"matchItems"`
	SuccItems   []*MatchItem `json:"succItems"`
	FailedItems []*MatchItem `json:"failedItems"`
}
type MatchItem struct {
	OrderHash string          `json:"orderHash"`
	Order     *Order          `json:"-"`
	Amount    decimal.Decimal `json:"amount"`
}

type MatchTransaction struct {
	ID                   string            `json:"id" db:"id" primaryKey:"true" gorm:"primary_key"`
	LiquidityPoolAddress string            `json:"liquidityPoolAddress" db:"liquidity_pool_address"`
	PerpetualIndex       int64             `json:"perpetualIndex" db:"perpetual_index"`
	BrokerAddress        string            `json:"brokerAddress" db:"broker_address"`
	MatchJson            string            `json:"-" db:"match_json"`
	Status               TransactionStatus `json:"status" db:"status"`
	BlockConfirmed       bool              `json:"blockConfirmed" db:"block_confirmed"`
	BlockNumber          null.Int          `json:"blockNumber" db:"block_number"`
	BlockHash            null.String       `json:"blockHash" db:"block_hash"`
	TransactionHash      null.String       `json:"transactionHash" db:"transaction_hash"`
	CreatedAt            time.Time         `json:"createdAt" db:"created_at"`
	ExecutedAt           null.Time         `json:"executedAt" db:"executed_at"`
	MatchResult          MatchResult       `json:"matchResult" sql:"-"`
}

func (MatchTransaction) TableName() string {
	return "match_transactions"
}
