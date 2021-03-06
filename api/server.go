package api

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/mcdexio/mai3-broker/common/chain"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"github.com/mcdexio/mai3-broker/match"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	logger "github.com/sirupsen/logrus"
)

type Server struct {
	ctx      context.Context
	e        *echo.Echo
	match    *match.Server
	chainCli chain.ChainClient
	dao      dao.DAO
}

func New(ctx context.Context, cli chain.ChainClient, dao dao.DAO, match *match.Server) (*Server, error) {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: `{"time":"${time_rfc3339}","remote_ip":"${remote_ip}","method":"${method}","uri":"${uri}","status":${status},"error":"${error}","request_time":"${latency_human}"}` + "\n",
	}))

	e.Use(RecoverHandler)
	e.HTTPErrorHandler = ErrorHandler
	e.Use(InitMaiApiContext)
	e.Use(GenericHTTPCollector())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			"Authentication",
			"Mai-Authentication",
		},
	}))

	s := &Server{
		ctx:      ctx,
		e:        e,
		match:    match,
		chainCli: cli,
		dao:      dao,
	}
	s.initRouter()
	return s, nil
}

func (s *Server) Start() error {
	srv := &http.Server{
		Addr:         conf.Conf.APIHost,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	srvFail := make(chan error, 1)
	go func() {
		if err := s.e.StartServer(srv); err != nil {
			srvFail <- err
		}
	}()

	select {
	case <-s.ctx.Done():
		logger.Infof("Api server shutdown")
		s.e.Listener.Close()
		// now close the server gracefully ("shutdown")
		graceTime := 10 * time.Second
		timeoutCtx, cancel := context.WithTimeout(context.Background(), graceTime)
		if err := s.e.Shutdown(timeoutCtx); err != nil {
			logger.Errorf("shutdown server error:%s", err.Error())
		}
		cancel()
	case err := <-srvFail:
		return err
	}
	return nil
}

func (s *Server) initRouter() {
	eg := s.e.Group("/orders", MaiAuthMiddleware, JwtAuthMiddleware, CheckAuthMiddleware)
	addGroupRoute(eg, "GET", "", &QueryOrderReq{}, s.GetOrders)
	addGroupRoute(eg, "GET", "/:orderHash", &QuerySingleOrderReq{}, s.GetOrderByOrderHash)
	addGroupRoute(eg, "POST", "/byhashs", &QueryOrdersByOrderHashsReq{}, s.GetOrdersByOrderHashs)
	addGroupRoute(eg, "DELETE", "/:orderHash", &CancelOrderReq{}, s.CancelOrder)
	addGroupRoute(eg, "DELETE", "", &CancelAllOrdersReq{}, s.CancelAllOrders)

	addRoute(s.e, "POST", "/orders", &PlaceOrderReq{}, s.PlaceOrder)
	addRoute(s.e, "GET", "/jwt", &BaseReq{}, GetJwtAuth, MaiAuthMiddleware, CheckAuthMiddleware)
	s.e.Add("GET", "/jwt2", CheckJwtAuthByCookie)
	addRoute(s.e, "GET", "/perpetuals/:perpetual", &GetPerpetualReq{}, s.GetPerpetual)
	addRoute(s.e, "GET", "/brokerRelay", &GetBrokerRelayReq{}, s.GetBrokerRelay)

	// metrics route
	s.e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
}

func addGroupRoute(eg *echo.Group, method, url string, param Param, handler func(p Param) (interface{}, error), middlewares ...echo.MiddlewareFunc) {
	eg.Add(method, url, CommonHandler(param, handler), middlewares...)
}

func addRoute(e *echo.Echo, method, url string, param Param, handler func(p Param) (interface{}, error), middlewares ...echo.MiddlewareFunc) {
	e.Add(method, url, CommonHandler(param, handler), middlewares...)
}
