package server

import (
	"fmt"
	"gb28181-proxy/data"
	"net"
	"strings"
)

func (s *SipProxy) startMediaServer() {
	proxyListen, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.config.ProxyIp, s.config.ProxyMediaPort))
	if err != nil {
		log.Errorf("Error listening on udp: %v", err)
		return
	}
	log.Infof("启动媒体服务: TCP:%s", proxyListen.Addr().String())
	go s.startMediaListener(proxyListen)
}

func (s *SipProxy) startMediaListener(proxyListen net.Listener) {
	defer proxyListen.Close()
	for {
		conn, err := proxyListen.Accept()
		if err != nil {
			log.Errorf("Error accepting udp connection: %v", err)
			return
		}
		deviceInfo := s.getDeviceInfo(strings.Split(conn.RemoteAddr().String(), ":")[0])
		if deviceInfo == nil {
			conn.Close()
			continue
		}
		log.Infof("媒体服务接收设备连接: TCP:%s", conn.RemoteAddr().String())
		go s.acceptMediaConnect(conn, deviceInfo)
	}
}

func (s *SipProxy) getDeviceInfo(ip string) *data.DeviceInfo {
	for _, device := range s.config.Devices {
		deviceInfo := s.proxyDeviceMap[device.SipUser]
		if deviceInfo == nil {
			continue
		}
		if deviceInfo.Host == ip {
			return deviceInfo
		}
	}
	return nil
}

func (s *SipProxy) acceptMediaConnect(conn net.Conn, deviceInfo *data.DeviceInfo) {
	defer conn.Close()
	mediaLocalHost := fmt.Sprintf("%s:%d", s.config.ClientIp, deviceInfo.StreamPort)
	mediaLocalAddr, err := net.ResolveTCPAddr("tcp", mediaLocalHost)
	if err != nil {
		log.Errorf("Error resolving tcp addr: %v", err)
		return
	}
	mediaServerHost := fmt.Sprintf("%s:%d", deviceInfo.MediaServerIp, deviceInfo.MediaServerPort)
	mediaServerAddr, err := net.ResolveTCPAddr("tcp", mediaServerHost)
	if err != nil {
		log.Errorf("Error resolving tcp addr: %v", err)
		return
	}
	mediaConn, err := net.DialTCP("tcp", mediaLocalAddr, mediaServerAddr)
	if err != nil {
		log.Errorf("Error listening on udp: %v", err)
		return
	}
	log.Infof("启动媒体服务推送: TCP:%s->TCP:%s->TCP:%s", conn.RemoteAddr().String(), mediaLocalHost, mediaServerHost)
	s.transfer(conn, mediaConn)
	log.Infof("关闭媒体服务推送: TCP:%s->TCP:%s->TCP:%s", conn.RemoteAddr().String(), mediaLocalHost, mediaServerHost)
}

func (s *SipProxy) transfer(in, out net.Conn) {
	defer func() {
		in.Close()
		out.Close()
	}()
	for {
		buffer := make([]byte, 1024)
		err := s.copyBuffer(in, out, buffer)
		if err != nil {
			log.Errorf("Error copying buffer: %v", err)
			return
		}
	}
}

func (s *SipProxy) copyBuffer(src, dest net.Conn, buffer []byte) error {
	n, err := src.Read(buffer)
	if err != nil {
		return err
	}
	_, err = dest.Write(buffer[:n])
	if err != nil {
		return err
	}
	return nil
}
