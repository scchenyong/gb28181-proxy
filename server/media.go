package server

import (
	"fmt"
	"gb28181-proxy/data"
	"io"
	"net"
	"strings"
	"time"
)

func (s *SipProxy) startMediaServer() {
	addr := fmt.Sprintf("%s:%d", s.config.ProxyIp, s.config.ProxyMediaPort)
	var proxyListen net.Listener
	var err error
	for {
		log.Infof("正在启动媒体服务: TCP:%s", addr)
		proxyListen, err = net.Listen("tcp", addr)
		if err != nil {
			log.Errorf("Media server start error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}
	log.Infof("完成媒体服务启动: TCP:%s", addr)
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
	for _, val := range s.proxyDeviceMap {
		if val.Host == ip {
			return val
		}
	}
	return nil
}

func (s *SipProxy) acceptMediaConnect(conn net.Conn, deviceInfo *data.DeviceInfo) {
	defer conn.Close()
	mediaServerHost := fmt.Sprintf("%s:%d", deviceInfo.MediaServerIp, deviceInfo.MediaServerPort)
	log.Infof("准备媒体服务推送: TCP:%s->TCP:%s->TCP:%s", conn.RemoteAddr().String(), conn.LocalAddr().String(), mediaServerHost)
	mediaConn, err := net.DialTimeout("tcp", mediaServerHost, 10*time.Second)
	if err != nil {
		log.Errorf("Error listening on tcp: %v", err)
		return
	}
	log.Infof("启动媒体服务推送: TCP:%s->TCP:%s->TCP:%s", conn.RemoteAddr().String(), conn.LocalAddr().String(), mediaServerHost)
	s.transfer(conn, mediaConn)
	log.Infof("关闭媒体服务推送: TCP:%s->TCP:%s->TCP:%s", conn.RemoteAddr().String(), conn.LocalAddr().String(), mediaServerHost)
}

func (s *SipProxy) transfer(in, out net.Conn) {
	defer func() {
		in.Close()
		out.Close()
	}()
	for {
		_, err := io.Copy(out, in)
		if err != nil {
			log.Errorf("Error copying buffer: %v", err)
			return
		}
	}
}
