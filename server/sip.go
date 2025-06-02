package server

import (
	"context"
	"errors"
	"gb28181-proxy/data"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/op/go-logging"
	"net"
)

const (
	UserAgent = "YSF-SIP/1.0"
)

var log = logging.MustGetLogger("ProxyServer")

type SipProxy struct {
	config *data.ProxyServer

	sipClientCtx context.Context
	proxyCtx     context.Context

	sipClientAgent *sipgo.UserAgent
	proxyAgent     *sipgo.UserAgent

	sipClientSender *sipgo.Client
	proxySender     *sipgo.Client

	sipClient *sipgo.Server

	proxyDeviceMap map[string]*data.DeviceInfo
	proxyServer    *sipgo.Server

	mediaListener net.Listener
}

func NewSipProxy(config *data.ProxyServer) (*SipProxy, error) {
	s := &SipProxy{
		sipClientCtx:   context.Background(),
		proxyCtx:       context.Background(),
		config:         config,
		proxyDeviceMap: make(map[string]*data.DeviceInfo),
	}
	return s, nil
}

func (s *SipProxy) Start() error {
	s.startClient()
	s.startProxy()
	s.startMediaServer()
	return nil
}

func (s *SipProxy) waitAnswer(ctx context.Context, tx sip.ClientTransaction) (*sip.Response, error) {
	select {
	case <-ctx.Done():
		return nil, errors.New("context done")
	case res := <-tx.Responses():
		if res.StatusCode == 100 || res.StatusCode == 101 || res.StatusCode == 180 || res.StatusCode == 183 {
			return s.waitAnswer(ctx, tx)
		}
		return res, nil
	}
}
