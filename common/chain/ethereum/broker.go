package ethereum

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethBind "github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/shopspring/decimal"

	"github.com/mcdexio/mai3-broker/common/chain/ethereum/broker"
	"github.com/mcdexio/mai3-broker/common/mai3"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/model"
)

func (c *Client) BatchTradeDataPack(compressedOrders [][]byte, matchAmounts []decimal.Decimal, gasRewards []*big.Int) ([]byte, error) {
	parsed, err := abi.JSON(strings.NewReader(broker.BrokerABI))
	if err != nil {
		return nil, err
	}
	amounts := make([]*big.Int, 0)
	for _, amount := range matchAmounts {
		amounts = append(amounts, utils.MustDecimalToBigInt(utils.ToWad(amount)))
	}
	inputs, err := parsed.Pack("batchTrade", compressedOrders, amounts, gasRewards)
	return inputs, err
}

func (c *Client) FilterTradeSuccess(ctx context.Context, brokerAddress string, start, end uint64) ([]*model.TradeSuccessEvent, error) {
	opts := &ethBind.FilterOpts{
		Start:   start,
		End:     &end,
		Context: ctx,
	}

	rsp := make([]*model.TradeSuccessEvent, 0)

	address, err := HexToAddress(brokerAddress)
	if err != nil {
		return rsp, fmt.Errorf("invalid broker address:%w", err)
	}

	contract, err := broker.NewBroker(address, c.GetEthClient())
	if err != nil {
		return rsp, fmt.Errorf("init broker contract failed:%w", err)
	}

	iter, err := contract.FilterTradeSuccess(opts)
	if err != nil {
		return rsp, fmt.Errorf("filter trade success event failed:%w", err)
	}

	for iter.Next() {
		match := &model.TradeSuccessEvent{
			PerpetualAddress: strings.ToLower(iter.Event.Raw.Address.Hex()),
			TransactionSeq:   int(iter.Event.Raw.TxIndex),
			TransactionHash:  strings.ToLower(iter.Event.Raw.TxHash.Hex()),
			BlockNumber:      int64(iter.Event.Raw.BlockNumber),
			TraderAddress:    strings.ToLower(iter.Event.Order.Trader.Hex()),
			OrderHash:        utils.Bytes2HexP(iter.Event.OrderHash[:]),
			Amount:           decimal.NewFromBigInt(iter.Event.Amount, -mai3.DECIMALS),
			Gas:              decimal.NewFromBigInt(iter.Event.GasReward, -mai3.DECIMALS),
		}

		rsp = append(rsp, match)
	}

	return rsp, nil
}

func (c *Client) FilterTradeFailed(ctx context.Context, brokerAddress string, start, end uint64) ([]*model.TradeFailedEvent, error) {
	opts := &ethBind.FilterOpts{
		Start:   start,
		End:     &end,
		Context: ctx,
	}

	rsp := make([]*model.TradeFailedEvent, 0)

	address, err := HexToAddress(brokerAddress)
	if err != nil {
		return rsp, fmt.Errorf("invalid broker address:%w", err)
	}

	contract, err := broker.NewBroker(address, c.GetEthClient())
	if err != nil {
		return rsp, fmt.Errorf("init broker contract failed:%w", err)
	}

	iter, err := contract.FilterTradeFailed(opts)
	if err != nil {
		return rsp, fmt.Errorf("filter trade failed event failed:%w", err)
	}

	for iter.Next() {
		match := &model.TradeFailedEvent{
			PerpetualAddress: strings.ToLower(iter.Event.Raw.Address.Hex()),
			TransactionSeq:   int(iter.Event.Raw.TxIndex),
			TransactionHash:  strings.ToLower(iter.Event.Raw.TxHash.Hex()),
			BlockNumber:      int64(iter.Event.Raw.BlockNumber),
			TraderAddress:    strings.ToLower(iter.Event.Order.Trader.Hex()),
			OrderHash:        utils.Bytes2HexP(iter.Event.OrderHash[:]),
			Amount:           decimal.NewFromBigInt(iter.Event.Amount, -mai3.DECIMALS),
			Reason:           iter.Event.Reason,
		}

		rsp = append(rsp, match)
	}

	return rsp, nil
}

func (c *Client) ParseLogs(brokerAddress string, logs []*ethtypes.Log) ([]*model.TradeSuccessEvent, []*model.TradeFailedEvent) {
	successEvents := make([]*model.TradeSuccessEvent, 0)
	failedEvents := make([]*model.TradeFailedEvent, 0)
	address, err := HexToAddress(brokerAddress)
	if err != nil {
		return successEvents, failedEvents
	}
	contract, err := broker.NewBroker(address, c.GetEthClient())
	if err != nil {
		return successEvents, failedEvents
	}
	for _, log := range logs {
		if event, err := contract.ParseTradeSuccess(*log); err == nil {
			success := &model.TradeSuccessEvent{
				PerpetualAddress: strings.ToLower(event.Raw.Address.Hex()),
				TransactionSeq:   int(event.Raw.TxIndex),
				TransactionHash:  strings.ToLower(event.Raw.TxHash.Hex()),
				BlockNumber:      int64(event.Raw.BlockNumber),
				TraderAddress:    strings.ToLower(event.Order.Trader.Hex()),
				OrderHash:        utils.Bytes2HexP(event.OrderHash[:]),
				Amount:           decimal.NewFromBigInt(event.Amount, -mai3.DECIMALS),
				Gas:              decimal.NewFromBigInt(event.GasReward, -mai3.DECIMALS),
			}
			successEvents = append(successEvents, success)
		}

		if event, err := contract.ParseTradeFailed(*log); err == nil {
			failed := &model.TradeFailedEvent{
				PerpetualAddress: strings.ToLower(event.Raw.Address.Hex()),
				TransactionSeq:   int(event.Raw.TxIndex),
				TransactionHash:  strings.ToLower(event.Raw.TxHash.Hex()),
				BlockNumber:      int64(event.Raw.BlockNumber),
				TraderAddress:    strings.ToLower(event.Order.Trader.Hex()),
				OrderHash:        utils.Bytes2HexP(event.OrderHash[:]),
				Amount:           decimal.NewFromBigInt(event.Amount, -mai3.DECIMALS),
				Reason:           event.Reason,
			}
			failedEvents = append(failedEvents, failed)
		}
	}
	return successEvents, failedEvents
}

func (c *Client) GetGasBalance(ctx context.Context, brokerAddress string, address string) (decimal.Decimal, error) {
	opts := &ethBind.CallOpts{
		Context: ctx,
	}

	var rsp decimal.Decimal

	account, err := HexToAddress(address)
	if err != nil {
		return rsp, fmt.Errorf("invalid user address:%w", err)
	}

	brokerAddr, err := HexToAddress(brokerAddress)
	if err != nil {
		return rsp, fmt.Errorf("invalid broker address:%w", err)
	}

	contract, err := broker.NewBroker(brokerAddr, c.GetEthClient())
	if err != nil {
		return rsp, fmt.Errorf("init broker contract failed:%w", err)
	}

	b, err := contract.BalanceOf(opts, account)
	if err != nil {
		return rsp, fmt.Errorf("read broker deposit gas balance failed:%w", err)
	}

	rsp = decimal.NewFromBigInt(b, -mai3.DECIMALS)
	return rsp, nil
}
