package launcher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mcdexio/mai3-broker/common/chain"
	"github.com/mcdexio/mai3-broker/common/mai3"
	mai3Utils "github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/mcdexio/mai3-broker/gasmonitor"
	"github.com/mcdexio/mai3-broker/match"
	"github.com/mcdexio/mai3-broker/runnable"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"math/big"
	"time"

	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
)

var ChannelHWM = 64

type Launcher struct {
	ctx        context.Context
	dao        dao.DAO
	runner     *runnable.Timed
	chainCli   chain.ChainClient
	gasMonitor *gasmonitor.GasMonitor
	match      *match.Server
	executor   *Executor
}

func NewLaunch(ctx context.Context, dao dao.DAO, chainCli chain.ChainClient, match *match.Server, gm *gasmonitor.GasMonitor) *Launcher {
	return &Launcher{
		ctx:        ctx,
		dao:        dao,
		runner:     runnable.NewTimed(ChannelHWM),
		chainCli:   chainCli,
		gasMonitor: gm,
		match:      match,
	}
}

func (l *Launcher) Start() error {
	logger.Infof("Launcher start")
	err := l.reloadAccount()
	if err != nil {
		logger.Errorf("reload account error %s", err)
		return err
	}

	// init transaction pending duration for each relayer account
	for _, addr := range l.chainCli.GetRelayerAccounts() {
		txPendingDuration.WithLabelValues(addr).Set(0)
	}

	group, ctx := errgroup.WithContext(l.ctx)

	// start syncer for sync pending transactions
	syncer := NewSyncer(ctx, l.dao, l.chainCli, l.match)
	group.Go(func() error {
		return syncer.Run()
	})

	// start monitor for check unmature transactions
	monitor := NewMonitor(ctx, l.dao, l.chainCli, l.match)
	group.Go(func() error {
		return monitor.Run()
	})

	// start executor for execute launch transactions
	executor := NewExecutor(ctx, l.dao, l.chainCli, l.gasMonitor, l.match)
	l.executor = executor
	group.Go(func() error {
		return executor.Run()
	})

	group.Go(func() error {
		return l.runner.Run(ctx, conf.Conf.LauncherInterval, l.checkMatchTransaction)
	})

	return group.Wait()
}

func (l *Launcher) reloadAccount() error {
	stores, err := l.dao.List("keystore")
	if err != nil {
		return err
	}
	for _, s := range stores {
		p, err := l.chainCli.DecryptKey(s.Value, conf.Conf.Password)
		if err != nil {
			return err
		}
		err = l.chainCli.AddAccount(p)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Launcher) ImportPrivateKey(pk string) (string, error) {
	p, address, err := l.chainCli.HexToPrivate(pk)
	if err != nil {
		return address, err
	}

	b, err := l.chainCli.EncryptKey(p, conf.Conf.Password)
	if err != nil {
		return address, fmt.Errorf("fail to encrypt key %s", err)
	}
	err = l.dao.Put(&model.KVStore{
		Key:      address,
		Category: "keystore",
		Value:    b,
	})
	if err != nil {
		return address, err
	}

	err = l.chainCli.AddAccount(p)
	return address, err
}

func (l *Launcher) checkMatchTransaction() {
	transactions, err := l.dao.QueryMatchTransaction("", 0, []model.TransactionStatus{model.TransactionStatusInit})
	if err != nil {
		logger.Errorf("QueryUnconfirmedTransactions failed error:%s", err)
		return
	}
	for _, transaction := range transactions {
		if err = l.dao.LoadMatchOrders(transaction.MatchResult.MatchItems); err != nil {
			logger.Errorf("LoadMatchOrders:%s", err)
			return
		}
	}

	for _, tx := range transactions {
		if err = l.createLaunchTransaction(tx); err != nil {
			logger.Errorf("createLaunchTransaction:%s", err)
			return
		}
	}
}

func (l *Launcher) createLaunchTransaction(matchTx *model.MatchTransaction) error {
	_, err := l.dao.GetTxByID(matchTx.ID)
	if !dao.IsRecordNotFound(err) {
		return errors.Errorf("Transaction already launched ID: %s", matchTx.ID)
	}

	orders := make([][]byte, 0)
	matchAmounts := make([]decimal.Decimal, 0)
	gasRewards := make([]*big.Int, 0)
	for _, item := range matchTx.MatchResult.MatchItems {
		data, err := GetCompressOrderData(item.Order)
		if err != nil {
			return err
		}
		orders = append(orders, data)
		matchAmounts = append(matchAmounts, item.Amount)
		gasReward := big.NewInt(0)
		if conf.Conf.GasEnable {
			gas := l.gasMonitor.GasPriceInWei().Mul(decimal.NewFromInt(item.Order.GasFeeLimit))
			// gas := mai3Utils.ToWad(decimal.NewFromFloat(0.0001))
			gasReward = mai3Utils.MustDecimalToBigInt(gas)
		}
		gasRewards = append(gasRewards, gasReward)
	}
	inputs, err := l.chainCli.BatchTradeDataPack(orders, matchAmounts, gasRewards)
	if err != nil {
		return err
	}

	signAccount, err := l.chainCli.GetSignAccount()
	if err != nil {
		return err
	}

	tx := &model.LaunchTransaction{
		TxID:        matchTx.ID,
		Type:        model.TxNormal,
		FromAddress: signAccount,
		ToAddress:   matchTx.BrokerAddress,
		Inputs:      inputs,
		Status:      model.TxInitial,
		CommitTime:  time.Now(),
	}

	err = l.dao.Transaction(l.ctx, false /* readonly */, func(dao dao.DAO) error {
		if err := dao.CreateTx(tx); err != nil {
			return fmt.Errorf("create transaction failed error:%w", err)
		}
		matchTx.Status = model.TransactionStatusPending
		if err := dao.UpdateMatchTransaction(matchTx); err != nil {
			return fmt.Errorf("update match transaction status failed error:%w", err)
		}
		return nil
	})

	if err == nil {
		l.executor.runner.Trigger(nil)
	}
	return err
}

func GetCompressOrderData(order *model.Order) ([]byte, error) {
	if order == nil {
		return nil, fmt.Errorf("GetCompressOrderData:nil order")
	}
	flags := mai3.GenerateOrderFlags(order.Type, order.IsCloseOnly)
	var orderSig model.OrderSignature
	err := json.Unmarshal([]byte(order.Signature), &orderSig)
	if err != nil {
		return nil, fmt.Errorf("GetCompressOrderData:%w", err)
	}
	orderData := mai3.GenerateOrderData(
		order.TraderAddress,
		order.BrokerAddress,
		order.RelayerAddress,
		order.ReferrerAddress,
		order.LiquidityPoolAddress,
		order.MinTradeAmount,
		order.Amount,
		order.Price,
		order.TriggerPrice,
		order.ChainID,
		order.ExpiresAt.UTC().Unix(),
		order.PerpetualIndex,
		order.BrokerFeeLimit,
		int64(flags),
		order.Salt,
		orderSig.SignType,
		orderSig.V,
		orderSig.R,
		orderSig.S,
	)

	logger.Infof("order_hash:%s, compress order data:%s", order.OrderHash, orderData)
	bytes, err := mai3Utils.Hex2Bytes(orderData)
	if err != nil {
		return nil, fmt.Errorf("GetCompressOrderData:%w", err)
	}
	return bytes, nil
}
