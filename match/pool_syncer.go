package match

import (
	"context"
	"time"

	"github.com/mcdexio/mai3-broker/common/chain"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/mcdexio/mai3-broker/conf"
	logger "github.com/sirupsen/logrus"

	"sync"
)

const POOL_STORAGE_USE_DURATION = 60

type poolInfo struct {
	storage   *model.LiquidityPoolStorage
	latestGet time.Time
	isDirty   bool
}

type poolSyncer struct {
	ctx         context.Context
	mu          sync.RWMutex
	poolStorage map[string]*poolInfo
	chainCli    chain.ChainClient
}

func newPoolSyncer(ctx context.Context, cli chain.ChainClient) *poolSyncer {
	return &poolSyncer{
		ctx:         ctx,
		chainCli:    cli,
		poolStorage: make(map[string]*poolInfo),
	}
}

func (p *poolSyncer) Run() error {
	var (
		before  time.Time
		elasped time.Duration
		timer   = time.NewTimer(0)
	)
	for {
		select {
		case <-p.ctx.Done():
			logger.Infof("pool syncer end")
			return nil
		case <-timer.C:
			before = time.Now()
			p.runSyncer()
			elasped = time.Since(before)
			if elasped >= conf.Conf.PoolSyncerInterval {
				timer.Reset(0)
			} else {
				timer.Reset(conf.Conf.PoolSyncerInterval - elasped)
			}
		}
	}
}

func (p *poolSyncer) runSyncer() error {
	for pool := range p.poolStorage {
		p.syncPool(pool)
	}
	return nil
}

func (p *poolSyncer) syncPool(pool string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	info, ok := p.poolStorage[pool]
	// pool storage not be used in duration, do not refresh it until someone used it again
	if ok && time.Since(info.latestGet).Seconds() > POOL_STORAGE_USE_DURATION {
		if !info.isDirty {
			info.isDirty = true
		}
		return
	}

	poolStorage, err := p.chainCli.GetLiquidityPoolStorage(p.ctx, conf.Conf.ReaderAddress, pool)
	if poolStorage == nil || err != nil {
		logger.Errorf("Pool Syncer: GetLiquidityPoolStorage fail! pool:%s err:%v", pool, err)
		info.storage = nil
		info.isDirty = true
	} else {
		info.storage = poolStorage
		info.isDirty = false
	}
}

func (p *poolSyncer) AddPool(pool string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.poolStorage[pool]; ok {
		return
	}
	poolStorage, err := p.chainCli.GetLiquidityPoolStorage(p.ctx, conf.Conf.ReaderAddress, pool)
	if poolStorage == nil || err != nil {
		logger.Errorf("Pool Syncer: GetLiquidityPoolStorage fail! pool:%s err:%v", pool, err)
	}
	p.poolStorage[pool] = &poolInfo{
		latestGet: time.Now(),
		isDirty:   false,
		storage:   poolStorage,
	}
}

func (p *poolSyncer) GetPoolStorage(pool string) *model.LiquidityPoolStorage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	info, ok := p.poolStorage[pool]
	if info == nil && !ok {
		return nil
	}
	if info.isDirty {
		// update latestGet, syncer will update PoolStorage in next round
		info.latestGet = time.Now()

		// get from chain
		poolStorage, err := p.chainCli.GetLiquidityPoolStorage(p.ctx, conf.Conf.ReaderAddress, pool)
		if poolStorage == nil || err != nil {
			return nil
		}
		return poolStorage
	}
	return info.storage
}
