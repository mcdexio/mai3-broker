package ethereum

import (
	"context"
	"github.com/ethereum/go-ethereum/accounts/abi"
	gethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
	"math/big"

	"github.com/mcarloai/mai-v3-broker/common/chain/ethereum/broker"
	"github.com/mcarloai/mai-v3-broker/common/model"
)

func (c *Client) BatchTradeDataPack(orderParams []*model.WalletOrderParam, matchAmounts []decimal.Decimal, gasRewards []*big.Int) ([]byte, error) {
	parsed, err := abi.JSON(strings.NewReader(broker.BrokerABI))
	if err != nil {
		return nil, err
	}
	orders := make([]broker.Order, len(orderParams))
	signatures := make([][]byte, len(orderParams))
	amounts := make([]*big.Int, len(orderParams))
	for _, param := range orderParams {
		order := broker.Order{
			Trader:      gethCommon.HexToAddress(param.Trader),
			Broker:      gethCommon.HexToAddress(param.Broker),
			Relayer:     gethCommon.HexToAddress(param.Relayer),
			Perpetual:   gethCommon.HexToAddress(param.Perpetual),
			Referrer:    gethCommon.HexToAddress(param.Referrer),
			Amount:      utils.MustDecimalToBigInt(utils.ToWad(param.Amount)),
			PriceLimit:  utils.MustDecimalToBigInt(utils.ToWad(param.Price)),
			Deadline:    param.Deadline,
			Version:     param.Version,
			OrderType:   param.OrderType,
			IsCloseOnly: param.IsCloseOnly,
			Salt:        param.Salt,
			ChainId:     param.ChainID,
		}
		orders = append(orders, order)
		signatures = append(signatures, param.Signature)
	}

	for _, amount := range matchAmounts {
		amounts = append(amounts, utils.MustDecimalToBigInt(utils.ToWad(amount)))
	}
	inputs, err := parsed.Pack("batchTrade", orders, amounts, signatures, gasRewards)
	return inputs, err
}

func (c *Client) FilterTradeSuccess(ctx context.Context, perpetualAddress string, start, end uint64) ([]*model.TradeSuccessEvent, error) {
	opts := &ethBind.FilterOpts{
		Start:   start,
		End:     &end,
		Context: ctx,
	}

	rsp := make([]*model.TradeSuccessEvent, 0)

	address, err := HexToAddress(perpetualAddress)
	if err != nil {
		return rsp, fmt.Errorf("invalid perpetual address:%w", err)
	}

	contract, err := perpetual.NewPerpetual(address, c.ethCli)
	if err != nil {
		return rsp, fmt.Errorf("init perpetual contract failed:%w", err)
	}

	iter, err := contract.FilterTradeSuccess(opts)
	if err != nil {
		return rsp, fmt.Errorf("filter trade event failed:%w", err)
	}

	if iter.Next() {
		match := &model.TradeSuccessEvent{
			PerpetualAddress: strings.ToLower(iter.Event.Raw.Address.Hex()),
			TransactionSeq:   int(iter.Event.Raw.TxIndex),
			TransactionHash:  strings.ToLower(iter.Event.Raw.TxHash.Hex()),
			BlockNumber:      int64(iter.Event.Raw.BlockNumber),
			TraderAddress:    strings.ToLower(iter.Event.Arg0.Trader.Hex()),
			OrderHash:        iter.Event.OrderHash.Hex(),
			Amount:           decimal.NewFromBigInt(iter.Event.Amount, -mai3.DECIMALS),
			Gas:              decimal.NewFromBigInt(iter.Event.GasReward, -mai3.DECIMALS),
		}

		rsp = append(rsp, match)
	}

	return rsp, nil
}