package launcher

import (
	"context"

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
	blockNumber, err := s.chainCli.GetLatestBlockNumber()
	if err != nil {
		logger.Infof("GetLatestBlockNumber error:%s", err)
		return
	}
	begin := blockNumber - conf.Conf.MatureBlockCount
	s.updateUnmatureTransactionStatus(&begin)
}

func (s *Monitor) updateUnmatureTransactionStatus(blockNumber *uint64) {
	// get unmature confirmed transaction
	logger.Debugf("check unmature transaction blockNumber > %d", *blockNumber)
	txs, err := s.dao.GetTxsByBlock(blockNumber, nil, model.TxSuccess, model.TxFailed)
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
			if tx.Status == model.TxSuccess {
				tx.Status = model.TxFailed
				err = s.match.RollbackOrdersStatus(tx.TxID, tx.Status.TransactionStatus(), *tx.TransactionHash, *tx.BlockHash, *tx.BlockNumber, *tx.BlockTime)
				if err != nil {
					logger.Warnf("transactionHash:%s not found, RollbackOrdersStatus fail txID: %s, err:%s", *tx.TransactionHash, tx.TxID, err)
					continue
				}
			}
		}
		if err != nil || receipt == nil {
			logger.Errorf("WaitTransactionReceipt error: %s", err)
			continue
		}

		// check blockNumber blockHash ?
		if tx.Status == receipt.Status {
			continue
		}
		err = s.dao.Transaction(context.Background(), false /* readonly */, func(dao dao.DAO) error {
			tx.BlockNumber = &receipt.BlockNumber
			tx.BlockHash = &receipt.BlockHash
			tx.BlockTime = &receipt.BlockTime
			tx.Status = receipt.Status
			tx.GasUsed = &receipt.GasUsed
			if err = dao.UpdateTx(tx); err != nil {
				return errors.Wrap(err, "fail to update transaction status")
			}

			err = s.match.RollbackOrdersStatus(tx.TxID, tx.Status.TransactionStatus(), *tx.TransactionHash, *tx.BlockHash, *tx.BlockNumber, *tx.BlockTime)
			return err
		})
		if err != nil {
			logger.Warnf("fail to check status: %s", err)
			return
		}
	}
}
