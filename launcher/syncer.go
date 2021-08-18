package launcher

import (
	"context"
	"sync"
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

type Syncer struct {
	ctx      context.Context
	dao      dao.DAO
	runner   *runnable.Timed
	chainCli chain.ChainClient
	match    *match.Server
}

func NewSyncer(ctx context.Context, dao dao.DAO, chainCli chain.ChainClient, match *match.Server) *Syncer {
	return &Syncer{
		ctx:      ctx,
		dao:      dao,
		runner:   runnable.NewTimed(ChannelHWM),
		chainCli: chainCli,
		match:    match,
	}
}

func (s *Syncer) Run() error {
	logger.Infof("Launcher syncer start")
	// sync pending transaction
	err := s.runner.Run(s.ctx, conf.Conf.SyncerInterval, s.syncTransaction)
	logger.Infof("Launcher syncer end")
	return err
}

func (s *Syncer) syncTransaction() {
	users, err := s.dao.GetUsersWithStatus(model.TxPending)
	if err != nil {
		logger.Warnf("find user with pending status failed: %s", err)
		return
	}
	wg := &sync.WaitGroup{}
	for _, user := range users {
		wg.Add(1)
		go func(user string) {
			defer wg.Done()
			s.updateStatusByUser(user)
		}(user)
	}
	wg.Wait()
}

func (s *Syncer) updateStatusByUser(user string) {
	logger.Debugf("check pending transaction for %s", user)
	txs, err := s.dao.GetTxsByUser(user, nil, model.TxPending)
	if dao.IsRecordNotFound(err) || len(txs) == 0 {
		return
	}
	if err != nil {
		logger.Infof("fail to get pending transaction err:%s", err)
		return
	}

	for i, tx := range txs {
		if tx.TransactionHash == nil {
			logger.Errorf("syncer: transaction hash is nill txID:%s", tx.TxID)
			return
		}

		txPendingDuration.WithLabelValues(tx.FromAddress).Set(float64(time.Since(tx.CommitTime).Milliseconds()))

		receipt, err := s.chainCli.WaitTransactionReceipt(*tx.TransactionHash)
		if s.chainCli.IsNotFoundError(err) {
			// this case is to handle accelarate
			if next := i + 1; next < len(txs) && *tx.Nonce == *txs[next].Nonce {
				continue
			}
			err = s.resetTransaction(tx)
			if err != nil {
				logger.Errorf("syncer: resetTransaction error: %s", err)
			}
			continue
		}
		if err != nil {
			logger.Errorf("syncer: WaitTransactionReceipt error: %s", err)
			continue
		}

		successEvents, failedEvents := s.chainCli.ParseLogs(conf.Conf.BrokerAddress, receipt.Logs)
		// if transaction success, there is one event at least
		if receipt.Status == model.TxSuccess && len(successEvents) == 0 && len(failedEvents) == 0 {
			logger.Errorf("Get Trade Event failed, neither success or failed: tx:%s, blocknumber:%d", tx.TxID, receipt.BlockNumber)
			continue
		}

		// set this pool storage dirty in match pool storage cache force to update it
		s.match.SetPoolStorageDirty(tx.TxID)

		err = s.dao.Transaction(s.ctx, false /* readonly */, func(dao dao.DAO) error {
			tx.BlockNumber = &receipt.BlockNumber
			tx.BlockHash = &receipt.BlockHash
			tx.BlockTime = &receipt.BlockTime
			tx.Status = receipt.Status
			tx.GasUsed = &receipt.GasUsed
			if err = dao.UpdateTx(tx); err != nil {
				return errors.Wrap(err, "fail to update transaction status")
			}
			// handle tx with same nonce
			candidates, err := dao.GetTxsByNonce(tx.FromAddress, tx.Nonce)
			if err != nil {
				return errors.Wrap(err, "fail to find transaction by nonce")
			}
			for _, candidate := range candidates {
				if *candidate.TransactionHash != *tx.TransactionHash {
					candidate.Status = model.TxCanceled
					dao.UpdateTx(candidate)
				}
			}

			err = s.match.UpdateOrdersStatus(tx.TxID, tx.Status.TransactionStatus(), *tx.TransactionHash, *tx.BlockHash, *tx.BlockNumber, *tx.BlockTime, successEvents, failedEvents)
			return err
		})

		if err != nil {
			logger.Warnf("fail to check status: %s", err)
			continue
		}
		txConfirmDuration.WithLabelValues(tx.FromAddress).Set(float64(time.Since(tx.CommitTime).Milliseconds()))
		txPendingDuration.WithLabelValues(tx.FromAddress).Set(0)
	}
}

func (s *Syncer) resetTransaction(tx *model.LaunchTransaction) error {
	err := s.dao.Transaction(s.ctx, false /* readonly */, func(dao dao.DAO) error {
		tx.Nonce = nil
		tx.TransactionHash = nil
		tx.Status = model.TxInitial
		err := dao.UpdateTx(tx)
		return err
	})
	return err
}
