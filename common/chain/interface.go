package chain

import (
	"context"
	"crypto/ecdsa"
	"math/big"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/shopspring/decimal"
)

type ChainClient interface {
	// account
	EncryptKey(pk *ecdsa.PrivateKey, password string) ([]byte, error)
	DecryptKey(cipher []byte, password string) (*ecdsa.PrivateKey, error)
	HexToPrivate(pk string) (*ecdsa.PrivateKey, string, error)
	AddAccount(private *ecdsa.PrivateKey) error
	GetSignAccount() (string, error)
	GetRelayerAccounts() []string

	// arb
	GetGasPrice(ctx context.Context, gasAddress string) ([]decimal.Decimal, error)

	// erc20
	BalanceOf(ctx context.Context, token, owner string, decimals int32) (decimal.Decimal, error)
	Allowance(ctx context.Context, token, owner, spender string, decimals int32) (decimal.Decimal, error)

	IsNotFoundError(err error) bool
	GetGasBalance(ctx context.Context, brokerAddress string, address string) (decimal.Decimal, error)
	GetLatestBlockNumber() (uint64, error)
	PendingNonceAt(account string) (uint64, error)
	TransactionByHash(txHash string) (bool, error)
	SendTransaction(tx *model.LaunchTransaction) (string, error)
	WaitTransactionReceipt(txHash string) (*model.Receipt, error)
	GetAccountStorage(ctx context.Context, readerAddress string, perpetualIndex int64, poolAddress, account string) (*model.AccountStorage, error)
	GetLiquidityPoolStorage(ctx context.Context, readerAddress, poolAddress string) (*model.LiquidityPoolStorage, error)
	FilterTradeSuccess(ctx context.Context, brokerAddress string, start, end uint64) ([]*model.TradeSuccessEvent, error)
	FilterTradeFailed(ctx context.Context, brokerAddress string, start, end uint64) ([]*model.TradeFailedEvent, error)
	BatchTradeDataPack(compressedOrders [][]byte, matchAmounts []decimal.Decimal, gasRewards []*big.Int) ([]byte, error)
	ParseLogs(brokerAddress string, logs []*ethtypes.Log) ([]*model.TradeSuccessEvent, []*model.TradeFailedEvent)
}
