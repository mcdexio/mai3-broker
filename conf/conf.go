package conf

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config for broker
type Config struct {
	APIHost       string   `envconfig:"api_host"`
	WebsocketHost string   `envconfig:"websocket_host"`
	DataBaseURL   string   `envconfig:"database_url"`
	JwtSecret     string   `envconfig:"jwt_secret"`
	ReaderAddress string   `envconfig:"reader_address"`
	BrokerAddress string   `envconfig:"broker_address"`
	WhiteList     []string `envconfig:"white_list"`
	EnableMetrics bool     `envconfig:"enable_metrics"`
	MaxOrderNum   int      `envconfig:"max_order_num"`

	PoolSyncerInterval      time.Duration `envconfig:"pool_syncer_interval"`
	MatchInterval           time.Duration `envconfig:"match_interval"`
	MatchMonitorInterval    time.Duration `envconfig:"match_monitor_interval"`
	LauncherInterval        time.Duration `envconfig:"launcher_interval"`
	ExecutorInterval        time.Duration `envconfig:"executor_interval"`
	SyncerInterval          time.Duration `envconfig:"syncer_interval"`
	LauncherMonitorInterval time.Duration `envconfig:"launcher_monitor_interval"`
	UnmatureDuration        time.Duration `envconfig:"unmature_duration"`

	GasPrice      uint64 `envconfig:"gas_price"`
	GasLimit      uint64 `envconfig:"gas_limit"`
	GasArbEnable  bool   `envconfig:"gas_arb_enable"`
	GasArbAddress string `envconfig:"gas_arb_address"`
	GasEnable     bool   `envconfig:"gas_enable"`

	ChainType    string            `envconfig:"chain_type"`
	ChainID      int64             `envconfig:"chain_id"`
	RetryTimes   int               `envconfig:"retry_times"`
	ChainTimeout time.Duration     `envconfig:"chain_timeout"`
	ProvidersURL []string          `envconfig:"providers_url"`
	Headers      map[string]string `envconfig:"headers"`
	Password     string            `envconfig:"password"`

	SubgraphURL string `envconfig:"subgraph_url"`
}

type L2RelayerConfig struct {
	BrokerAddress    string        `envconfig:"broker_address"`
	ProviderURL      string        `envconfig:"provider_url"`
	ChainID          int64         `envconfig:"chain_id"`
	GasPrice         uint64        `envconfig:"gas_price"`
	L2RelayerHost    string        `envconfig:"l2_relayer_host"`
	L2Timeout        time.Duration `envconfig:"l2_timeout"`
	L2CallFeePercent uint32        `envconfig:"l2_call_fee_percent"`
	L2RelayerKey     string        `envconfig:"l2_relayer_key"`
}

var (
	// Conf for server
	Conf          Config
	L2RelayerConf L2RelayerConfig
)

// Init is parse config file
func Init() (err error) {
	return envconfig.Process("mcdex", &Conf)
}

func InitL2RelayerConf() (err error) {
	return envconfig.Process("mcdex", &L2RelayerConf)
}
