package server

import (
	"fmt"
	"gb28181-proxy/data"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"net"
	"net/http"
	"strings"
	"time"
)

func (s *SipProxy) startProxy() {
	agent, err := sipgo.NewUA(sipgo.WithUserAgent(UserAgent))
	if err != nil {
		log.Errorf("NewUA err: %v", err)
		return
	}
	s.proxyAgent = agent
	client, err := sipgo.NewClient(s.proxyAgent)
	if err != nil {
		log.Errorf("NewClient err: %v", err)
		return
	}
	s.proxySender = client
	server, err := sipgo.NewServer(s.proxyAgent)
	if err != nil {
		log.Errorf("Sip proxy server start error: %v", err)
		return
	}
	s.proxyServer = server
	s.proxyServer.OnRegister(func(req *sip.Request, tx sip.ServerTransaction) {
		res := s.onProxyRegister(req)
		log.Infof("ProxyServer OnRegister: Source=%s, CSeq=%s, StatusCode=%d, Reason=%s", req.Source(), req.CSeq().String(), res.StatusCode, res.Reason)
		err = tx.Respond(res)
		if err != nil {
			log.Errorf("Sip TransactionResponse error: %v", err)
			return
		}
		return
	})
	s.proxyServer.OnMessage(func(req *sip.Request, tx sip.ServerTransaction) {
		res := s.proxyMessage(req)
		log.Infof("ProxyServer OnMessage: Source=%s, CSeq=%s, StatusCode=%d, Reason=%s", req.Source(), req.CSeq().String(), res.StatusCode, res.Reason)
		err = tx.Respond(res)
		if err != nil {
			log.Errorf("Sip TransactionResponse error: %v", err)
			return
		}
		return
	})
	s.proxyServer.OnBye(s.onProxyBye)

	addr := fmt.Sprintf("%s:%d", s.config.ProxyIp, s.config.ProxySipPort)
	if !s.config.DisableProxyUdp {
		go func() {
			for {
				log.Infof("启动服务端: UDP:%s", addr)
				err = s.proxyServer.ListenAndServe(s.proxyCtx, "udp", addr)
				if err != nil {
					log.Errorf("Sip proxy server start udp error: %v", err)
					time.Sleep(5 * time.Second)
					continue
				}
			}
		}()
	}
	if !s.config.DisableProxyTcp {
		go func() {
			for {
				log.Infof("启动服务端: TCP:%s", addr)
				err = s.proxyServer.ListenAndServe(s.proxyCtx, "tcp", addr)
				if err != nil {
					log.Errorf("Sip proxy server start tcp error: %v", err)
					time.Sleep(5 * time.Second)
					continue
				}
			}
		}()
	}
}

func (s *SipProxy) onProxyRegister(req *sip.Request) *sip.Response {
	request := s.sipProxy2ClientRequest(req)
	if request == nil {
		return sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
	}
	res, err := s.sipClientSender.Do(s.proxyCtx, request)
	if err != nil {
		log.Errorf("Sip TransactionRequest error: %v", err)
		return sip.NewResponseFromRequest(req, http.StatusInternalServerError, "", nil)
	}
	if res.StatusCode == sip.StatusOK {
		user := req.From().Address.User
		deviceInfo, ok := s.proxyDeviceMap[user]
		if !ok {
			deviceInfo = &data.DeviceInfo{
				Id:       user,
				Protocol: req.Transport(),
				Host:     req.Via().Host,
				Port:     req.Via().Port,
			}
			s.proxyDeviceMap[user] = deviceInfo
			deviceAddr := fmt.Sprintf("%s:%s:%d", deviceInfo.Protocol, deviceInfo.Host, deviceInfo.Port)
			log.Infof("设备注册成功: %s->%s:%s:%d", deviceAddr, deviceInfo.Protocol, s.config.ProxyIp, s.config.ProxySipPort)
		}
	}

	response := sip.NewResponseFromRequest(req, res.StatusCode, res.Reason, res.Body())
	for _, header := range res.Headers() {
		if response.GetHeader(header.Name()) != nil {
			continue
		}
		response.AppendHeader(header)
	}
	return response
}

func (s *SipProxy) proxyMessage(req *sip.Request) *sip.Response {
	request := s.sipProxy2ClientRequest(req)
	if request == nil {
		return sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
	}
	user := req.From().Address.User
	_, ok := s.proxyDeviceMap[user]
	if !ok {
		return sip.NewResponseFromRequest(req, http.StatusUnauthorized, "", nil)
	}
	res, err := s.sipClientSender.Do(s.proxyCtx, request)
	if err != nil {
		log.Errorf("Sip TransactionRequest error: %v", err)
		return sip.NewResponseFromRequest(req, http.StatusInternalServerError, "", nil)
	}

	response := sip.NewResponseFromRequest(req, res.StatusCode, res.Reason, res.Body())
	for _, header := range res.Headers() {
		if response.GetHeader(header.Name()) != nil {
			continue
		}
		response.AppendHeader(header)
	}
	return response
}

func (s *SipProxy) onProxyBye(req *sip.Request, tx sip.ServerTransaction) {
	log.Debugf("ProxyBye: CSeq=%s, Call-ID=%s", req.CSeq().String(), req.CallID())
	request := s.sipProxy2ClientRequest(req)
	if request == nil {
		return
	}
	res, err := s.sipClientSender.Do(s.proxyCtx, request)
	if err != nil {
		log.Errorf("Sip TransactionRequest error: %v", err)
		return
	}
	response := sip.NewResponseFromRequest(req, res.StatusCode, res.Reason, res.Body())
	for _, header := range res.Headers() {
		if response.GetHeader(header.Name()) != nil {
			continue
		}
		response.AppendHeader(header)
	}
	err = tx.Respond(response)
	if err != nil {
		log.Errorf("Sip TransactionResponse error: %v", err)
		return
	}
}

func (s *SipProxy) sipProxy2ClientRequest(req *sip.Request) *sip.Request {
	clientReq := req.Clone()

	via := clientReq.Via()
	if via != nil {
		via.Host = s.config.ClientIp
		via.Port = s.config.ClientPort
		clientReq.ReplaceHeader(via)
	}

	contact := clientReq.Contact()
	if contact != nil {
		contact.Address.Host = s.config.ClientIp
		contact.Address.Port = s.config.ClientPort
		clientReq.ReplaceHeader(contact)
	}

	clientReq.SetTransport(strings.ToUpper(s.config.ServerProtocol))
	clientReq.SetSource(fmt.Sprintf(fmt.Sprintf("%s:%d", s.config.ClientIp, s.config.ClientPort)))
	clientReq.SetDestination(fmt.Sprintf("%s:%d", s.config.ServerIp, s.config.ServerPort))
	clientReq.SetBody(req.Body())
	if strings.ToUpper(clientReq.Transport()) == "UDP" {
		clientReq.Laddr = sip.Addr{IP: net.ParseIP(s.config.ClientIp), Port: s.config.ClientPort}
	}

	return clientReq
}
