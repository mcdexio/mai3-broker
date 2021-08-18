package launcher

import (
	"context"
	"time"

	"github.com/mcdexio/mai3-broker/common/chain"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/mcdexio/mai3-broker/match"
	"github.com/mcdexio/mai3-broker/runnable"
	"github.com/pkg/errors"
	logger "github.com/sirupsen/logrus"
)

type Monitor struct {
	ctx      context.Context
	dao      dao.DAO
	runner   *runnable.Timed
	chainCli chain.ChainClient
	match    *match.Server
}

func NewMonitor(ctx context.Context, dao dao.DAO, chainCli chain.ChainClient, match *match.Server) *Monitor {
	return &Monitor{
		ctx:      ctx,
		dao:      dao,
		runner:   runnable.NewTimed(ChannelHWM),
		chainCli: chainCli,
		match:    match,
	}
}

func (s *Monitor) Run() error {
	logger.Infof("Launcher monitor start")
	// check unmature confirmed transaction for rollback
	err := s.runner.Run(s.ctx, conf.Conf.LauncherMonitorInterval, s.syncUnmatureTransaction)
	logger.Infof("Launcher monitor end")
	return err
}

func (s *Monitor) syncUnmatureTransaction() {
	s.checkUnmatureTransactionStatus()
}

func (s *Monitor) checkUnmatureTransactionStatus() {
	unmatureTime := time.Now().UTC().Add(-conf.Conf.UnmatureDuration)
	// get unmature confirmed transaction
	logger.Debugf("check unmature transaction commit_time after %s", unmatureTime)
	txs, err := s.dao.GetTxsByTime(unmatureTime, model.TxSuccess, model.TxFailed)
	if dao.IsRecordNotFound(err) || len(txs) == 0 {
		return
	}
	if err != nil {
		logger.Infof("fail to get pending transaction err:%s", err)
		return
	}

	for _, tx := range txs {
		if tx.TransactionHash == nil || tx.BlockNumber == nil {
			logger.Errorf("transaction hash or blockNumber is nill txID:%s", tx.TxID)
			continue
		}

		receipt, err := s.chainCli.WaitTransactionReceipt(*tx.TransactionHash)
		if s.chainCli.IsNotFoundError(err) {
			// transaction hash not found, rollback all success orders
			if tx.Status == model.TxSuccess {
				err = s.dao.Transaction(s.ctx, false /* readonly */, func(dao dao.DAO) error {
					tx.Status = model.TxFailed
					if err = dao.UpdateTx(tx); err != nil {
						return errors.Wrap(err, "fail to update transaction status")
					}
					err = s.match.RollbackOrdersStatus(tx.TxID, tx.Status.TransactionStatus(), *tx.TransactionHash, *tx.BlockHash, *tx.BlockNumber, *tx.BlockTime, []*model.TradeSuccessEvent{}, []*model.TradeFailedEvent{})
					return err
				})
				if err != nil {
					logger.Errorf("rollback transaction failed! txID:%s, err:%s", tx.TxID, err)
				}
				continue
			}
		}

		if err != nil || receipt == nil {
			logger.Errorf("checkUnmatureTransactionStatus WaitTransactionReceipt error: %s", err)
			continue
		}

		successEvents, failedEvents := s.chainCli.ParseLogs(conf.Conf.BrokerAddress, receipt.Logs)
		// if transaction success, there is one event at least
		if receipt.Status == model.TxSuccess && len(successEvents) == 0 && len(failedEvents) == 0 {
			logger.Errorf("Get Trade Event failed, neither success or failed: tx:%s, blocknumber:%d", tx.TxID, receipt.BlockNumber)
			continue
		}

		err = s.dao.Transaction(s.ctx, false /* readonly */, func(dao dao.DAO) error {
			if *tx.BlockNumber != receipt.BlockNumber || *tx.BlockHash != receipt.BlockHash || tx.Status != receipt.Status {
				tx.BlockNumber = &receipt.BlockNumber
				tx.BlockHash = &receipt.BlockHash
				tx.BlockTime = &receipt.BlockTime
				tx.Status = receipt.Status
				tx.GasUsed = &receipt.GasUsed
				if err = dao.UpdateTx(tx); err != nil {
					return errors.Wrap(err, "fail to update transaction status")
				}
			}

			err = s.match.RollbackOrdersStatus(tx.TxID, tx.Status.TransactionStatus(), *tx.TransactionHash, *tx.BlockHash, *tx.BlockNumber, *tx.BlockTime, successEvents, failedEvents)
			return err
		})
		if err != nil {
			logger.Warnf("fail to check status: %s", err)
		}
	}
}
