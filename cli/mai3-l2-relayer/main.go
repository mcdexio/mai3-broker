package main

import (
	"context"
	"flag"
	"math/big"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/l2/relayer"
	"github.com/mcdexio/mai3-broker/l2/rpc"
	"golang.org/x/sync/errgroup"

	logger "github.com/sirupsen/logrus"
)

func main() {
	backgroundCtx, stop := context.WithCancel(context.Background())
	go WaitExitSignal(stop)

	group, ctx := errgroup.WithContext(backgroundCtx)

	var err error
	flag.Parse()
	if err := conf.InitL2RelayerConf(); err != nil {
		panic(err)
	}

	r, err := relayer.NewRelayer(
		conf.L2RelayerConf.ProviderURL,
		big.NewInt(conf.L2RelayerConf.ChainID),
		conf.L2RelayerConf.L2RelayerKey,
		conf.L2RelayerConf.BrokerAddress,
		conf.L2RelayerConf.GasPrice,
		conf.L2RelayerConf.L2CallFeePercent)

	if err != nil {
		logger.Errorf("create l2 relayer fail:%s", err.Error())
		os.Exit(-5)
	}

	// start api server
	server, err := rpc.NewL2RelayerServer(ctx, r)
	if err != nil {
		logger.Errorf("create l2 relayer server fail:%s", err.Error())
		os.Exit(-5)
	}

	group.Go(func() error {
		return server.Start()
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
