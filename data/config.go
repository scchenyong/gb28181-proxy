package data

type ProxyServer struct {
	ServerIp        string `json:"serverIp"`
	ServerPort      int    `json:"serverPort"`
	ServerProtocol  string `json:"serverProtocol"`
	ClientIp        string `json:"clientIp"`
	ClientPort      int    `json:"clientPort"`
	ProxyIp         string `json:"proxyIp"`
	ProxySipPort    int    `json:"proxySipPort"`
	ProxyMediaPort  int    `json:"proxyMediaPort"`
	DisableProxyUdp bool   `json:"disableProxyUdp"`
	DisableProxyTcp bool   `json:"disableProxyTcp"`
}
