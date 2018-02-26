package main

import (
	"qiniu.com/dora-cloud/boots/controller/ha"
	"qiniu.com/dora-cloud/boots/controller/ha/example/mockhttp"

	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/qiniu/http/restrpc.v1"
	"github.com/qiniu/http/servestk.v1"
	jsonlog "qbox.us/http/audit/jsonlog.v7"
	"qbox.us/servestk/gracedown"
	"qiniu.com/apps/rpc"
)

var (
	auditlogCfg = jsonlog.Config{
		LogFile:   "./auditlog/",
		ChunkBits: 29,
	}
	gracedownCfg = gracedown.Config{
		TimeoutRules: []gracedown.TimeoutRule{
			gracedown.TimeoutRule{
				Left:   5,
				Expire: 60,
			},
		},
		On: 1,
	}
	confPath string
)

// 该 config 是 example 特有的 config
type Config struct {
	*ha.Service         `json:"service"`
	*ha.ConsulConfig    `json:"consul_config"`
	*ha.HAManagerConfig `json:"ha_manager_config"`
	LogLevel            string `json:"log_level"`
	ReleaseDelaySeconds int    `json:"release_delay_seconds"`
}

func (c Config) String() string {
	return `Config: {Service: ` + c.Service.String() + `, ConsulConfig: ` + c.ConsulConfig.String() +
	`, HAManagerConfig: ` + c.HAManagerConfig.String() + `, LogLevel: ` + c.LogLevel +
	`, ReleaseDelaySeconds: ` + strconv.Itoa(c.ReleaseDelaySeconds) + `}`
}

func load(filePath string, config *Config) error {
	log.Debugf("file path[%s]", filePath)
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}

func main() {
	flag.StringVar(&confPath, "f", "./example1.conf", "configure file")
	flag.Parse()

	var cfg Config
	err := load(confPath, &cfg)
	if err != nil {
		log.Fatal("fail to load config file")
	} else {
		log.Infof("config is [%s]\n", cfg)
	}

	l, _ := log.ParseLevel(cfg.LogLevel)
	log.SetLevel(l)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetOutput(os.Stdout)

	runServer(cfg)
	time.Sleep(10000 * time.Second)
}

//func runServer(region string, consulAddrs []string, delay int, service ha.Service) {
func runServer(cfg Config) {
	hamgr := ha.NewHAManager(cfg.ConsulConfig, cfg.HAManagerConfig)
	hamgr.Register(cfg.Service)

	al, logf, err := jsonlog.Open("BOOTS-CONTROLLER", &auditlogCfg)
	if err != nil {
		log.Fatalf("Failed to open jsonlog, err: %v\n", err)
	}
	defer logf.Close()
	gd := gracedown.NewStatus(&gracedownCfg)
	mux := servestk.New(restrpc.DefaultServeMux, gd.Handler(), al.Handler, hamgr.Handler)
	router := restrpc.Router{
		PatternPrefix: "v1",
		Factory:       rpc.Factory,
		Mux:           mux,
	}
	ctr := mockhttp.NewMockCrtl()
	router.Register(ctr)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Service.Port)
		err = http.ListenAndServe(addr, mux.Mux)
		log.Fatal("Service exit ...", err)
	}()

	err = hamgr.Setup()
	if err != nil {
		log.Fatalf("fail to setup HA manager, err: %s", err)
	}

	hamgr.Start()

	fmt.Printf("delay is %d\n", cfg.ReleaseDelaySeconds)
	time.Sleep(time.Duration(cfg.ReleaseDelaySeconds) * time.Second)
	hamgr.Stop()
}
