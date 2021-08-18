package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcdexio/mai3-broker/dao"

	"github.com/mcdexio/mai3-broker/api"
	"github.com/mcdexio/mai3-broker/common/chain"
	"github.com/mcdexio/mai3-broker/common/chain/ethereum"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/gasmonitor"
	"github.com/mcdexio/mai3-broker/launcher"
	"github.com/mcdexio/mai3-broker/match"
	"github.com/mcdexio/mai3-broker/perpetualsyncer"
	"github.com/mcdexio/mai3-broker/websocket"
	"golang.org/x/sync/errgroup"

	logger "github.com/sirupsen/logrus"
)

func main() {
	logger.SetOutput(os.Stdout)
	backgroundCtx, stop := context.WithCancel(context.Background())
	go WaitExitSignal(stop)

	group, ctx := errgroup.WithContext(backgroundCtx)

	var err error
	flag.Parse()
	if err := conf.Init(); err != nil {
		panic(err)
	}

	// init database
	if err = dao.ConnectPostgres(conf.Conf.DataBaseURL); err != nil {
		logger.Errorf("create database fail:%s", err.Error())
		os.Exit(-1)
	}

	dao := dao.New()

	var chainCli chain.ChainClient
	if conf.Conf.ChainType == "ethereum" {
		chainCli, err = ethereum.NewClient(ctx, conf.Conf.ProvidersURL, conf.Conf.ChainTimeout, conf.Conf.RetryTimes, conf.Conf.Headers)
		if err != nil {
			logger.Errorf("init ethereum client error:%s", err.Error())
			os.Exit(-2)
		}
	}

	// gas monitor for fetch gas price
	gasMonitor := gasmonitor.NewGasMonitor(ctx, chainCli)
	group.Go(func() error {
		return gasMonitor.Run()
	})

	// perpetual syncer for sync perpetuals from mcdex subgraph
	perpetualSyncer, err := perpetualsyncer.NewPerpetualSyncer(ctx, dao)
	if err != nil {
		logger.Errorf("NewPerpetualSyncer fail:%s", err)
		os.Exit(-3)
	}
	group.Go(func() error {
		return perpetualSyncer.Run()
	})

	// msg chan for websocket message
	wsChan := make(chan interface{}, 100)

	// new match server
	matchServer, err := match.New(ctx, chainCli, dao, wsChan, gasMonitor)
	if err != nil {
		logger.Errorf("new match server error:%s", err)
		os.Exit(-4)
	}
	group.Go(func() error {
		return matchServer.Start()
	})

	// start api server
	apiServer, err := api.New(ctx, chainCli, dao, matchServer)
	if err != nil {
		logger.Errorf("create api server fail:%s", err.Error())
		os.Exit(-5)
	}

	group.Go(func() error {
		return apiServer.Start()
	})

	// start websocket server
	wsServer := websocket.New(ctx, wsChan)
	group.Go(func() error {
		return wsServer.Start()
	})

	// start launcher
	launch := launcher.NewLaunch(ctx, dao, chainCli, matchServer, gasMonitor)
	group.Go(func() error {
		return launch.Start()
	})

	if err := group.Wait(); err != nil {
		logger.Fatalf("service stopped: %s", err)
	}
}

func WaitExitSignal(ctxStop context.CancelFunc) {
	var exitSignal = make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGTERM)
	signal.Notify(exitSignal, syscall.SIGINT)

	sig := <-exitSignal
	logger.Infof("caught sig: %+v, Stopping...\n", sig)
	ctxStop()
}
