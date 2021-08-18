package launcher

import (
	"context"

	"github.com/mcdexio/mai3-broker/common/chain"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/mcdexio/mai3-broker/gasmonitor"
	"github.com/mcdexio/mai3-broker/match"
	"github.com/mcdexio/mai3-broker/runnable"
	"github.com/pkg/errors"
	logger "github.com/sirupsen/logrus"
)

type Executor struct {
	ctx        context.Context
	dao        dao.DAO
	runner     *runnable.Timed
	match      *match.Server
	chainCli   chain.ChainClient
	gasMonitor *gasmonitor.GasMonitor
}

func NewExecutor(ctx context.Context, dao dao.DAO, chainCli chain.ChainClient, gm *gasmonitor.GasMonitor, match *match.Server) *Executor {
	return &Executor{
		ctx:        ctx,
		dao:        dao,
		runner:     runnable.NewTimed(ChannelHWM),
		chainCli:   chainCli,
		gasMonitor: gm,
		match:      match,
	}
}

func (s *Executor) Run() error {
	logger.Infof("Launcher Executor start")
	err := s.runner.Run(s.ctx, conf.Conf.ExecutorInterval, s.executeTransaction)
	logger.Infof("Launcher Executor end")
	return err
}

func (s *Executor) executeTransaction() {
	users, err := s.dao.GetUsersWithStatus(model.TxInitial)
	if err != nil {
		logger.Warnf("find user with status failed: %s", err)
		return
	}
	for _, user := range users {
		tx, err := s.dao.FirstTxByUser(user, model.TxInitial)
		if err != nil {
			logger.Warningf("retreive new transaction failed for user %v", user)
			continue
		}
		logger.Infof("found initial transaction %s from user %s", tx.TxID, user)
		err = s.sendTransaction(tx)
		if err != nil {
			logger.Warningf("commit new transaction failed for %v: %v", user, err)
		}
	}
}

func (s *Executor) sendTransaction(tx *model.LaunchTransaction) error {
	logger.Infof("send transaction for user %v", tx.FromAddress)
	expNonce, err := s.chainCli.PendingNonceAt(tx.FromAddress)
	if err != nil {
		return errors.Wrap(err, "access node rpc failed")
	}
	// has nonce, but external transaction
	if tx.Nonce != nil && expNonce > *tx.Nonce {
		logger.Warnf("found external transactions, try reset current transaction (CANCEL)")
		if err := s.reset(tx); err != nil {
			return errors.Wrap(err, "reset transaction failed")
		}
		return nil
	}
	minNonce, ok := s.dao.GetNextNonce(tx.FromAddress)
	if (tx.Nonce != nil && expNonce < *tx.Nonce) || (tx.Nonce == nil && ok && expNonce < minNonce) {
		var targetNonce uint64
		if tx.Nonce != nil {
			targetNonce = *tx.Nonce
		} else {
			targetNonce = minNonce
		}
		logger.Warnf("found expect nonce lower than assigned, try fast-forward [%v, %v]", expNonce, targetNonce)
		if err = s.fastForward(tx.FromAddress, expNonce, targetNonce); err != nil {
			logger.Warnf("fast-forward failed, will try to send transation: %s", err)
		}
		logger.Infof("try fast-forward done")
		return nil
	}
	// allocate nonce
	if err := s.dao.Transaction(s.ctx, false /* readonly */, func(dao dao.DAO) error {
		tx.Nonce = model.Uint64(expNonce)
		if err := dao.UpdateTx(tx); err != nil {
			return errors.Wrap(err, "save nonce failed")
		}
		if err := dao.UpdateNonce(tx.FromAddress, model.Uint64(*tx.Nonce+1)); err != nil {
			return errors.Wrap(err, "update nonce failed")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "allocate nonce failed")
	}
	logger.Infof("using nonce %v", *tx.Nonce)
	// send
	if err := s.send(tx); err != nil {
		return errors.Wrap(err, "send transaction failed")
	}
	return nil
}

func (s *Executor) reset(tx *model.LaunchTransaction) error {
	if tx.TransactionHash == nil {
		tx.Status = model.TxCanceled
	} else {
		_, err := s.chainCli.TransactionByHash(*tx.TransactionHash)
		if err != nil {
			tx.Status = model.TxCanceled
		} else {
			tx.Status = model.TxPending
		}
	}
	err := s.dao.Transaction(s.ctx, false /* readonly */, func(dao dao.DAO) error {
		if err := dao.UpdateTx(tx); err != nil {
			return errors.Wrap(err, "update transaction failed")
		}
		err := s.match.UpdateOrdersStatus(tx.TxID, tx.Status.TransactionStatus(), "", "", 0, 0, []*model.TradeSuccessEvent{}, []*model.TradeFailedEvent{})
		return err
	})

	return err
}

func (s *Executor) fastForward(addr string, start uint64, end uint64) error {
	for i := start; i < end; i++ {
		logger.Infof("resend transaction by nonce %v", i)
		prevs, err := s.dao.GetTxsByNonce(addr, model.Uint64(i))
		if err != nil {
			return errors.Wrap(err, "find previous transaction failed")
		}
		if len(prevs) == 0 {
			continue
		}
		for _, tx := range prevs {
			if err := s.send(tx); err != nil {
				logger.Warnf("(may no be a issue) try resending previous transaction %v failed, err %v", tx.TxID, err)
			}
		}
	}
	return nil
}

// prepare transaction price and gas limit
func (s *Executor) prepare(tx *model.LaunchTransaction) error {
	if tx.Nonce == nil {
		return errors.New("missing nonce")
	}

	limit := conf.Conf.GasLimit
	tx.GasLimit = &limit
	price := s.gasMonitor.GasPriceInWei().BigInt().Uint64()
	tx.GasPrice = &price
	return nil
}

func (s *Executor) send(tx *model.LaunchTransaction) error {
	return s.dao.Transaction(s.ctx, false /* readonly */, func(dao dao.DAO) error {
		prevHash := tx.TransactionHash
		err := s.prepare(tx)
		if err != nil {
			return errors.Wrap(err, "prepare transaction failed")
		}
		currHash, err := s.chainCli.SendTransaction(tx)
		if err != nil {
			return errors.Wrap(err, "send transaction failed")
		}
		tx.Status = model.TxPending
		tx.TransactionHash = &currHash
		if prevHash != nil && model.MustString(prevHash) != currHash {
			logger.Infof("resend tx, former tx: %s => %s", model.MustString(prevHash), currHash)
			tx.ID = 0
			if err := dao.CreateTx(tx); err != nil {
				return errors.Wrap(err, "create resend transaction failed")
			}
		} else {
			logger.Infof("update tx, tx: %s", currHash)
			if err := dao.UpdateTx(tx); err != nil {
				return errors.Wrap(err, "update transaction failed")
			}
		}
		return nil
	})
}
