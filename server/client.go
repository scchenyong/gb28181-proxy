package server

import (
	"fmt"
	"gb28181-proxy/data"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"net"
	"net/http"
	"strconv"
	"strings"
)

func (s *SipProxy) startClient() {
	agent, err := sipgo.NewUA(sipgo.WithUserAgent(UserAgent))
	if err != nil {
		log.Errorf("NewUA err: %v", err)
		return
	}
	s.sipClientAgent = agent
	client, err := sipgo.NewClient(s.sipClientAgent)
	if err != nil {
		log.Errorf("NewClient err: %v", err)
		return
	}
	s.sipClientSender = client
	server, err := sipgo.NewServer(s.sipClientAgent)
	if err != nil {
		log.Errorf("Sip proxy server start error: %v", err)
		return
	}
	s.sipClient = server
	s.sipClient.OnMessage(s.onSipClientMessage)
	s.sipClient.OnInvite(s.OnSipClientInvite)
	s.sipClient.OnAck(s.OnSipClientAck)
	s.sipClient.OnBye(s.OnSipClientBye)

	clientAddr := fmt.Sprintf("%s:%d", s.config.ClientIp, s.config.ClientPort)
	go func() {
		proto := strings.ToUpper(s.config.ServerProtocol)
		serverAddr := fmt.Sprintf("%s:%s:%d", proto, s.config.ServerIp, s.config.ServerPort)
		log.Infof("启动客户端: %s:%s->%s", proto, clientAddr, serverAddr)
		err = s.sipClient.ListenAndServe(s.proxyCtx, s.config.ServerProtocol, clientAddr)
		if err != nil {
			log.Errorf("Sip proxy server start error: %v", err)
			return
		}
	}()
}

func (s *SipProxy) OnSipClientInvite(req *sip.Request, tx sip.ServerTransaction) {
	log.Debug("SipClientInvite:", req.String())
	user := req.Recipient.User

	device, ok := s.proxyDeviceMap[user]
	if !ok {
		resp := sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Error responding to invite: %v", err)
		}
		return
	}

	request := s.sipClient2ProxyRequest(req, device)
	sdpInfo, mediaIp, medialPort := s.sdpInfoParse(request.Body(), s.config.ProxyIp, s.config.ProxyMediaPort)
	request.SetBody(sdpInfo)

	device.MediaServerIp = mediaIp
	device.MediaServerPort = medialPort

	log.Debug("ProxyInvite:", request.String())
	transaction, err := s.proxySender.TransactionRequest(s.proxyCtx, request)
	if err != nil {
		log.Errorf("Error proxying invite: %v", err)
		return
	}

	res, err := s.waitAnswer(s.proxyCtx, transaction)
	if err != nil {
		log.Errorf("Error proxying invite: %v", err)
		return
	}

	resSdpInfo, _, _ := s.sdpInfoParse(res.Body(), s.config.ClientIp, device.StreamPort)
	response := sip.NewResponseFromRequest(req, res.StatusCode, res.Reason, nil)
	response.SetBody(resSdpInfo)

	err = tx.Respond(response)
	if err != nil {
		log.Errorf("Sip TransactionResponse error: %v", err)
		return
	}
	log.Debug("ProxyInviteResponse:", response.String())

	ackReq := sip.NewRequest(sip.ACK, request.Recipient)
	for _, header := range request.Headers() {
		ackReq.AppendHeader(header)
	}
	ackReq.SetTransport(request.Transport())
	ackReq.SetDestination(request.Destination())
	err = s.proxySender.WriteRequest(ackReq)
	if err != nil {
		log.Errorf("Sip TransactionResponse error: %v", err)
		return
	}
}

func (s *SipProxy) OnSipClientAck(req *sip.Request, tx sip.ServerTransaction) {
	log.Debug("SipClientAck:", req.String())
}

func (s *SipProxy) OnSipClientBye(req *sip.Request, tx sip.ServerTransaction) {
	log.Debug("SipClientBye:", req.String())
	user := req.Recipient.User

	device, ok := s.proxyDeviceMap[user]
	if !ok {
		resp := sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Error responding to invite: %v", err)
		}
		return
	}

	request := s.sipClient2ProxyRequest(req, device)
	res, err := s.proxySender.Do(s.proxyCtx, request)
	if err != nil {
		log.Errorf("Sip TransactionResponse error: %v", err)
		return
	}

	err = tx.Respond(sip.NewResponseFromRequest(req, res.StatusCode, "", nil))
	if err != nil {
		log.Errorf("Sip TransactionResponse error: %v", err)
		return
	}
}

func (s *SipProxy) onSipClientMessage(req *sip.Request, tx sip.ServerTransaction) {
	log.Debug("SipClientMessage:", req.String())
	user := req.Recipient.User

	device, ok := s.proxyDeviceMap[user]
	if !ok {
		resp := sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil)
		err := tx.Respond(resp)
		if err != nil {
			log.Errorf("Error responding to invite: %v", err)
		}
		return
	}
	request := s.sipClient2ProxyRequest(req, device)
	res, err := s.proxySender.Do(s.proxyCtx, request)
	if err != nil {
		log.Errorf("Sip TransactionRequest error: %v, %s", err, request.String())
		return
	}

	response := sip.NewResponseFromRequest(req, res.StatusCode, res.Reason, res.Body())
	err = tx.Respond(response)
	if err != nil {
		log.Errorf("Sip TransactionResponse error: %v", err)
		return
	}
}

func (s *SipProxy) sdpInfoParse(sdpInfo []byte, proxyMediaHost string, proxyMedialPort int) (newSdpInfo []byte, sipMediaHost string, sipMediaPort int) {
	sdpL := strings.Split(string(sdpInfo), "\n")
	var newSdp []string
	for _, sdp := range sdpL {
		sdp = strings.TrimSpace(sdp)
		if strings.HasPrefix(sdp, "o=") || strings.HasPrefix(sdp, "c=") {
			sdpA := strings.Split(sdp, " ")
			sdpLen := len(sdpA)
			if len(sipMediaHost) == 0 {
				sipMediaHost = sdpA[sdpLen-1]
			}
			sdpA[sdpLen-1] = proxyMediaHost
			newSdp = append(newSdp, strings.Join(sdpA, " "))
			continue
		}
		if strings.HasPrefix(sdp, "m=") {
			sdpA := strings.Split(sdp, " ")
			sipMediaPort, _ = strconv.Atoi(strings.TrimSpace(sdpA[1]))
			sdpA[1] = fmt.Sprintf("%d", proxyMedialPort)
			newSdp = append(newSdp, strings.Join(sdpA, " "))
			continue
		}
		newSdp = append(newSdp, sdp)
	}
	newSdp = append(newSdp, "\r\n")
	newSdpInfo = []byte(strings.Join(newSdp, "\r\n"))
	return
}

func (s *SipProxy) sipClient2ProxyRequest(req *sip.Request, device *data.DeviceInfo) *sip.Request {
	proxyReq := req.Clone()

	proxyReq.Recipient.Host = device.Host
	proxyReq.Recipient.Port = device.Port

	via := proxyReq.Via()
	if via != nil {
		via.Host = s.config.ProxyIp
		via.Port = s.config.ProxySipPort
		proxyReq.ReplaceHeader(via)
	}

	contact := proxyReq.Contact()
	if contact != nil {
		contact.Address.Host = s.config.ProxyIp
		contact.Address.Port = s.config.ProxySipPort
		proxyReq.ReplaceHeader(contact)
	}

	proxyReq.SetTransport(device.Protocol)
	proxyReq.SetSource(fmt.Sprintf("%s:%d", s.config.ProxyIp, s.config.ProxySipPort))
	proxyReq.SetDestination(fmt.Sprintf("%s:%d", device.Host, device.Port))
	proxyReq.SetBody(req.Body())
	if strings.ToUpper(proxyReq.Transport()) == "UDP" {
		proxyReq.Laddr = sip.Addr{
			IP:   net.ParseIP(s.config.ProxyIp),
			Port: s.config.ProxySipPort,
		}
	}
	return proxyReq
}
