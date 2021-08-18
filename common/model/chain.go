package model

import (
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/shopspring/decimal"
)

type BlockHeader struct {
	BlockNumber int64
	BlockHash   string
	ParentHash  string
	BlockTime   time.Time
}

type Receipt struct {
	BlockNumber uint64
	BlockHash   string
	GasUsed     uint64
	Status      LaunchTransactionStatus
	BlockTime   uint64
	Logs        []*ethtypes.Log
}

type TradeSuccessEvent struct {
	PerpetualAddress string
	BlockNumber      int64
	TransactionSeq   int
	TransactionHash  string
	TraderAddress    string
	OrderHash        string
	Amount           decimal.Decimal
	Gas              decimal.Decimal
}

type TradeFailedEvent struct {
	PerpetualAddress string
	BlockNumber      int64
	TransactionSeq   int
	TransactionHash  string
	TraderAddress    string
	OrderHash        string
	Amount           decimal.Decimal
	Reason           string
}
