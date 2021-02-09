package match

import (
	"context"
	"fmt"
	"github.com/mcarloai/mai-v3-broker/common/chain"
	"github.com/mcarloai/mai-v3-broker/common/model"
	"github.com/mcarloai/mai-v3-broker/dao"
	"github.com/mcarloai/mai-v3-broker/gasmonitor"
	"github.com/shopspring/decimal"
	logger "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"sync"
)

type Server struct {
	ctx             context.Context
	mu              sync.Mutex
	matchHandlerMap map[string]*match
	wsChan          chan interface{}
	matchErrChan    chan error
	chainCli        chain.ChainClient
	gasMonitor      *gasmonitor.GasMonitor
	dao             dao.DAO
	group           *errgroup.Group
}

func New(ctx context.Context, cli chain.ChainClient, dao dao.DAO, wsChan chan interface{}, gm *gasmonitor.GasMonitor) (*Server, error) {
	server := &Server{
		ctx:             ctx,
		wsChan:          wsChan,
		matchHandlerMap: make(map[string]*match),
		matchErrChan:    make(chan error, 1),
		chainCli:        cli,
		gasMonitor:      gm,
		dao:             dao,
	}

	group, _ := errgroup.WithContext(ctx)
	server.group = group
	return server, nil
}

func (s *Server) Start() error {
	logger.Infof("Match Server Start")
	perpetuals, err := s.dao.QueryPerpetuals(true)
	if err != nil {
		logger.Errorf("New Match Server QueryPerpetuals:%w", err)
		return err
	}

	for _, perpetual := range perpetuals {
		match, err := s.newMatch(perpetual)
		if err != nil {
			logger.Errorf("New SubMatch Server newMatch:%w", err)
			return err
		}
		s.group.Go(func() error {
			return match.Run()
		})
	}

	err = s.group.Wait()
	logger.Infof("Match Server End err:%s", err)
	return err
}

func (s *Server) newMatch(perpetual *model.Perpetual) (*match, error) {
	m := newMatch(s.ctx, s.chainCli, s.dao, perpetual, s.wsChan, s.gasMonitor)
	err := s.setMatchHandler(perpetual.LiquidityPoolAddress, perpetual.PerpetualIndex, m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Server) NewOrder(order *model.Order) string {
	handler := s.getMatchHandler(order.LiquidityPoolAddress, order.PerpetualIndex)
	if handler == nil {
		perpetual, err := s.dao.GetPerpetualByPoolAddressAndIndex(order.LiquidityPoolAddress, order.PerpetualIndex, true)
		if err != nil {
			logger.Errorf("new order: get perpetual error:%s", err)
			return model.MatchInternalErrorID
		}
		handler, err = s.newMatch(perpetual)
		if err != nil {
			logger.Errorf("new order: new match error:%s", err)
			return model.MatchInternalErrorID
		}
		s.group.Go(func() error {
			return handler.Run()
		})
	}
	return handler.NewOrder(order)
}

func (s *Server) CancelOrder(poolAddress string, perpetualIndex int64, orderHash string) error {
	handler := s.getMatchHandler(poolAddress, perpetualIndex)
	if handler == nil {
		return fmt.Errorf("CancelOrder error: perpetual[%s-%d] is not open.", poolAddress, perpetualIndex)
	}
	return handler.CancelOrder(orderHash, model.CancelReasonUserCancel, true, decimal.Zero)
}

func (s *Server) ClosePerpetual(poolAddress string, perpIndex int64) error {
	handler := s.getMatchHandler(poolAddress, perpIndex)
	if handler == nil {
		return fmt.Errorf("ClosePerpetual error: perpetual[%s-%d] is not open.", poolAddress, perpIndex)
	}
	perpetual, err := s.dao.GetPerpetualByPoolAddressAndIndex(poolAddress, perpIndex, true)
	if err != nil {
		return err
	}
	perpetual.IsPublished = false
	err = s.dao.UpdatePerpetual(perpetual)
	if err != nil {
		return err
	}
	handler.Close()
	return s.deleteMatchHandler(poolAddress, perpIndex)
}

func (s *Server) UpdateOrdersStatus(txID string, status model.TransactionStatus, transactionHash, blockHash string, blockNumber, blockTime uint64) error {
	matchTx, err := s.dao.GetMatchTransaction(txID)
	if err != nil {
		return err
	}
	handler := s.getMatchHandler(matchTx.LiquidityPoolAddress, matchTx.PerpetualIndex)
	if handler == nil {
		return fmt.Errorf("UpdateOrdersStatus error: perpetual[%s-%d] is not open.", matchTx.LiquidityPoolAddress, matchTx.PerpetualIndex)
	}
	err = handler.UpdateOrdersStatus(txID, status, transactionHash, blockHash, blockNumber, blockTime)
	return err
}

func (s *Server) RollbackOrdersStatus(txID string, status model.TransactionStatus, transactionHash, blockHash string, blockNumber, blockTime uint64) error {
	matchTx, err := s.dao.GetMatchTransaction(txID)
	if err != nil {
		return err
	}
	handler := s.getMatchHandler(matchTx.LiquidityPoolAddress, matchTx.PerpetualIndex)
	if handler == nil {
		return fmt.Errorf("RollBackOrdersStatus error: perpetual[%s-%d] is not open.", matchTx.LiquidityPoolAddress, matchTx.PerpetualIndex)
	}
	err = handler.RollbackOrdersStatus(txID, status, transactionHash, blockHash, blockNumber, blockTime)
	return err
}

func getPerpetualKey(liquidityPoolAddress string, perpIndex int64) string {
	return fmt.Sprintf("%s-%d", liquidityPoolAddress, perpIndex)
}

func (s *Server) getMatchHandler(liquidityPoolAddress string, perpIndex int64) *match {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := getPerpetualKey(liquidityPoolAddress, perpIndex)
	h, ok := s.matchHandlerMap[key]
	if !ok {
		return nil
	}
	return h
}

func (s *Server) setMatchHandler(liquidityPoolAddress string, perpIndex int64, h *match) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := getPerpetualKey(liquidityPoolAddress, perpIndex)
	_, ok := s.matchHandlerMap[key]
	if ok {
		return fmt.Errorf("setMatchHandler error:liquidityPoolAddress[%s-%d] exists", liquidityPoolAddress, perpIndex)
	}
	s.matchHandlerMap[key] = h
	return nil
}

func (s *Server) deleteMatchHandler(liquidityPoolAddress string, perpIndex int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := getPerpetualKey(liquidityPoolAddress, perpIndex)
	_, ok := s.matchHandlerMap[key]
	if ok {
		delete(s.matchHandlerMap, key)
	} else {
		return fmt.Errorf("deleteMatchHandler:perpetual[%s] do not exists", key)
	}
	return nil
}
