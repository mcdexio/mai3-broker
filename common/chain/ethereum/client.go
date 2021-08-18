package ethereum

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum"
	ethCommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"math/big"
	"math/rand"
	"sync"

	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/pkg/errors"
)

type Client struct {
	ctx         context.Context
	ethClis     []*ethclient.Client
	accounts    map[ethCommon.Address]*Account //accounts for sign transaction
	aliases     []string
	callTimeout time.Duration
	tryTimes    int
	mu          sync.RWMutex
}

func NewClient(ctx context.Context, providers []string, timeout time.Duration, tryTimes int, headers map[string]string) (*Client, error) {
	cli := &Client{
		ctx:         ctx,
		ethClis:     make([]*ethclient.Client, 0),
		accounts:    make(map[ethCommon.Address]*Account),
		callTimeout: timeout,
		tryTimes:    tryTimes,
	}
	for _, provider := range providers {
		rpcClient, err := rpc.Dial(provider)
		if err != nil {
			return nil, err
		}

		for key, value := range headers {
			rpcClient.SetHeader(key, value)
		}

		cli.ethClis = append(cli.ethClis, ethclient.NewClient(rpcClient))
	}

	if len(cli.ethClis) < cli.tryTimes {
		cli.tryTimes = len(cli.ethClis)
	}

	return cli, nil
}

func (c *Client) GetEthClient() *ethclient.Client {
	rand.Seed(time.Now().Unix())
	cliLen := len(c.ethClis)
	if cliLen == 0 {
		return nil
	}
	idx := rand.Intn(cliLen)
	return c.ethClis[idx]
}

func (c *Client) IsNotFoundError(err error) bool {
	if errors.Is(err, ethereum.NotFound) {
		return true
	}
	return false
}

func (c *Client) call(method string, args ...interface{}) (interface{}, error) {
	cliLen := len(c.ethClis)
	if cliLen == 0 {
		return nil, fmt.Errorf("no client for call")
	}

	var loopErr error
	rand.Seed(time.Now().Unix())
	idx := rand.Intn(cliLen)
	for i := 0; i < c.tryTimes; i++ {
		ethCli := c.ethClis[idx]
		ctx, cancel := context.WithTimeout(c.ctx, c.callTimeout)
		defer cancel()
		switch method {
		case "PendingNonceAt":
			nonce, err := ethCli.PendingNonceAt(ctx, args[0].(ethCommon.Address))
			if err == nil {
				return nonce, nil
			}
			loopErr = err
		case "BlockByNumber":
			var blockNumber *big.Int
			if args[0] != nil {
				blockNumber = args[0].(*big.Int)
			}
			block, err := ethCli.BlockByNumber(ctx, blockNumber)
			if err == nil {
				return block, nil
			}
			loopErr = err
		case "TransactionByHash":
			_, isPending, err := ethCli.TransactionByHash(ctx, args[0].(ethCommon.Hash))
			if err == nil {
				return isPending, nil
			}
			loopErr = err
		case "SendTransaction":
			err := ethCli.SendTransaction(ctx, args[0].(*ethtypes.Transaction))
			if err == nil {
				return nil, nil
			}
			loopErr = err
		case "TransactionReceipt":
			rcpt, err := ethCli.TransactionReceipt(ctx, args[0].(ethCommon.Hash))
			if err == nil {
				return rcpt, nil
			}
			loopErr = err
		default:
			return nil, fmt.Errorf("unsupport method %s", method)
		}

		if !errors.Is(loopErr, context.DeadlineExceeded) {
			return nil, loopErr
		}

		idx++
		if idx == cliLen {
			idx = 0
		}
	}

	return nil, loopErr
}

func (c *Client) GetSignAccount() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	accLen := len(c.aliases)
	if accLen == 0 {
		return "", errors.New("no account added")
	}

	rand.Seed(time.Now().Unix())
	idx := rand.Intn(accLen)
	return c.aliases[idx], nil
}

func (c *Client) GetRelayerAccounts() []string {
	return c.aliases
}

func (c *Client) AddAccount(private *ecdsa.PrivateKey) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	account, err := PrivateToAccount(private, big.NewInt(conf.Conf.ChainID))
	if err != nil {
		return err
	}
	if _, ok := c.accounts[account.Address()]; ok {
		return errors.New("account duplicated")
	}
	c.accounts[account.Address()] = account
	c.aliases = append(c.aliases, account.String())
	return nil
}

func (c *Client) GetAccount(account string) (*Account, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	address := ethCommon.HexToAddress(account)
	acc, ok := c.accounts[address]
	if !ok {
		return nil, errors.New("account not exists")
	}
	return acc, nil
}

func (c *Client) GetLatestBlockNumber() (uint64, error) {
	block, err := c.call("BlockByNumber", nil)
	if err != nil {
		return 0, errors.Wrap(err, "fail to get latest block number")
	}

	return block.(*ethtypes.Block).NumberU64(), nil
}

func (c *Client) PendingNonceAt(account string) (uint64, error) {
	address := ethCommon.HexToAddress(account)
	nonce, err := c.call("PendingNonceAt", address)
	if err != nil {
		return 0, err
	}
	return nonce.(uint64), nil
}

func (c *Client) TransactionByHash(txHash string) (bool, error) {
	transactionHash := ethCommon.HexToHash(txHash)
	isPending, err := c.call("TransactionByHash", transactionHash)
	if err != nil {
		return false, err
	}
	return isPending.(bool), nil
}

func (c *Client) SendTransaction(tx *model.LaunchTransaction) (string, error) {
	rawTx := ethtypes.NewTransaction(
		*tx.Nonce,
		ethCommon.HexToAddress(tx.ToAddress),
		utils.MustDecimalToBigInt(tx.Value),
		*tx.GasLimit,
		big.NewInt(int64(*tx.GasPrice)),
		tx.Inputs)

	acc, err := c.GetAccount(tx.FromAddress)
	if err != nil {
		return "", err
	}
	signedTx, err := acc.Signer().Signer(acc.Address(), rawTx)
	if err != nil {
		return "", err
	}
	_, err = c.call("SendTransaction", signedTx)
	if err != nil {
		return "", err
	}

	return signedTx.Hash().Hex(), nil
}

func (c *Client) WaitTransactionReceipt(txHash string) (*model.Receipt, error) {
	var res interface{}
	var err error
	for i := 0; i < 10; i++ {
		res, err = c.call("TransactionReceipt", ethCommon.HexToHash(txHash))
		if errors.Is(err, ethereum.NotFound) {
			sleep := time.Millisecond * 300 * time.Duration(i+1)
			time.Sleep(sleep)
			continue
		} else {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	rcpt := res.(*ethtypes.Receipt)
	if rcpt.BlockNumber == nil {
		return nil, errors.New("empty block number")
	}

	block, err := c.call("BlockByNumber", rcpt.BlockNumber)
	if err != nil {
		return nil, errors.Wrap(err, "fail to retrieve block for transaction")
	}

	receipt := &model.Receipt{
		BlockNumber: rcpt.BlockNumber.Uint64(),
		BlockHash:   rcpt.BlockHash.Hex(),
		GasUsed:     rcpt.GasUsed,
		BlockTime:   block.(*ethtypes.Block).Time(),
		Logs:        rcpt.Logs,
	}
	if rcpt.Status == ethtypes.ReceiptStatusSuccessful {
		receipt.Status = model.TxSuccess
	} else {
		receipt.Status = model.TxFailed
	}
	return receipt, nil
}
