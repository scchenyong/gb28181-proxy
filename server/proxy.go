package server

import (
	"fmt"
	"gb28181-proxy/data"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"net"
	"net/http"
	"strings"
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
	s.proxyServer.OnRegister(s.onProxyRegister)
	s.proxyServer.OnMessage(s.onProxyMessage)
	s.proxyServer.OnBye(s.onProxyBye)

	addr := fmt.Sprintf("%s:%d", s.config.ProxyIp, s.config.ProxySipPort)
	if !s.config.DisableProxyUdp {
		go func() {
			log.Infof("启动服务端: UDP:%s", addr)
			err = s.proxyServer.ListenAndServe(s.proxyCtx, "udp", addr)
			if err != nil {
				log.Errorf("Sip proxy server start error: %v", err)
				return
			}
		}()
	}
	if !s.config.DisableProxyTcp {
		go func() {
			log.Infof("启动服务端: TCP:%s", addr)
			err = s.proxyServer.ListenAndServe(s.proxyCtx, "tcp", addr)
			if err != nil {
				log.Errorf("Sip proxy server start error: %v", err)
				return
			}
		}()
	}
}

func (s *SipProxy) onProxyRegister(req *sip.Request, tx sip.ServerTransaction) {
	log.Debug("ProxyRegister:", req.String())
	request := s.sipProxy2ClientRequest(req)
	if request == nil {
		resp := sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Sip TransactionResponse error: %v", err)
			return
		}
		return
	}
	user := req.From().Address.User
	var configDevice *data.DeviceConfig
	for _, device := range s.config.Devices {
		if user == device.SipUser {
			configDevice = device
			break
		}
	}
	if configDevice == nil {
		resp := sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Error responding to invite: %v", err)
		}
		return
	}

	res, err := s.sipClientSender.Do(s.proxyCtx, request)
	if err != nil {
		log.Errorf("Sip TransactionRequest error: %v", err)
		return
	}
	if res.StatusCode == sip.StatusOK {
		deviceInfo, ok := s.proxyDeviceMap[user]
		if !ok {
			deviceInfo = &data.DeviceInfo{
				Id:         user,
				Protocol:   req.Transport(),
				Host:       req.Via().Host,
				Port:       req.Via().Port,
				StreamPort: configDevice.StreamPort,
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
	err = tx.Respond(response)
	if err != nil {
		log.Errorf("Sip TransactionResponse error: %v", err)
		return
	}
}

func (s *SipProxy) onProxyMessage(req *sip.Request, tx sip.ServerTransaction) {
	log.Debug("ProxyMessage:", req.String())
	request := s.sipProxy2ClientRequest(req)
	if request == nil {
		resp := sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Sip TransactionResponse error: %v", err)
			return
		}
		return
	}

	user := req.From().Address.User
	var configDevice *data.DeviceConfig
	for _, device := range s.config.Devices {
		if user == device.SipUser {
			configDevice = device
			break
		}
	}
	if configDevice == nil {
		resp := sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Error responding to invite: %v", err)
		}
		return
	}

	_, ok := s.proxyDeviceMap[req.From().Address.User]
	if !ok {
		resp := sip.NewResponseFromRequest(req, http.StatusUnauthorized, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Sip TransactionResponse error: %v", err)
			return
		}
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

func (s *SipProxy) onProxyBye(req *sip.Request, tx sip.ServerTransaction) {
	log.Debug("ProxyBye:", req.String())
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
