# gb28181-proxy

## 说明
### 本项目解决跨网络GB28181协议代理转发和视频流转发


## 配置
```json
{
  "serverIp": "192.168.123.100",
  "serverPort": 5060,
  "serverProtocol": "tcp",
  "clientIp": "192.168.123.55",
  "clientPort": 5060,
  "proxyIp": "192.168.1.55",
  "proxySipPort": 5060,
  "proxyMediaPort": 5678
}
```

## 网络
```mermaid
flowchart LR
    A[摄像头A<br>192.168.1.11] -->|SIP+RTP| C[代理接入<br>192.168.1.55]
    B[摄像头B<br>192.168.1.12] -->|SIP+RTP| C
    subgraph 代理服务器
    C --> D[代理推送<br>192.168.123.55]
    end
    D -->|SIP| E[信令服务器<br>192.168.123.100]
    D -->|RTP| F[媒体服务器<br>192.168.123.200]
```