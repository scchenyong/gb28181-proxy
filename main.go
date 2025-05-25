package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"gb28181-proxy/data"
	"gb28181-proxy/server"
	"github.com/op/go-logging"
	syslog "log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

var (
	_config string
	_debug  bool
)

func init() {
	flag.StringVar(&_config, "key", "", "配置文件")
	flag.BoolVar(&_debug, "debug", false, "开启调试模式")
	flag.Usage = usage

	logging.SetBackend(logging.NewLogBackend(os.Stdout, "", syslog.LstdFlags|syslog.Lmicroseconds))
	logging.SetFormatter(logging.MustStringFormatter("%{message}"))
	logging.SetLevel(logging.DEBUG, "")
}

func main() {
	flag.Parse()

	backend := logging.NewLogBackend(os.Stdout, "", syslog.LstdFlags|syslog.Lmicroseconds)
	logging.SetBackend(backend)
	logging.SetFormatter(logging.MustStringFormatter(
		`%{module} %{shortfile} %{level} %{message}`,
	))
	logLevel := logging.INFO
	if _debug {
		logLevel = logging.DEBUG
	}
	logging.SetLevel(logLevel, "")

	log := logging.MustGetLogger("gtaf-sip-proxy")

	var rc *data.ProxyServer
	if len(_config) == 0 {
		runDir, _ := os.Executable()
		dirPath := filepath.Dir(runDir)
		_config = fmt.Sprintf("%s/config.json", dirPath)
	}
	readFile, err := os.ReadFile(_config)
	if err != nil {
		log.Errorf("载入配置文件[%s]出错! 错误: %v", _config, err)
		os.Exit(-1)
	}

	err = json.Unmarshal(readFile, &rc)
	if nil != err {
		log.Errorf("解析配置文件内容出错! 错误: %v", err)
		os.Exit(-1)
	}

	marshal, _ := json.Marshal(rc)
	log.Debug(string(marshal))

	proxy, err := server.NewSipProxy(rc)
	if err != nil {
		log.Errorf("创建代理服务出错! 错误: %v", err)
		os.Exit(-1)
	}
	err = proxy.Start()
	if nil != err {
		log.Errorf("启动代理服务出错! 错误: %v", err)
		os.Exit(-1)
	}
	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-signalChan

	os.Exit(0)
}

func usage() {
	fmt.Fprint(os.Stdout, `主程序版本: v1.0
用法: proxy [-config] [-debug]
选项:`)
	fmt.Fprintln(os.Stdout)
	flag.PrintDefaults()
}
