package config

import "net"

func DetectOutboundIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return "127.0.0.1"
	}
	return addr.IP.String()
}

func (c *Config) ExternalHost() string {
	if c.Server.ExternalHost != "" {
		return c.Server.ExternalHost
	}
	if c.Server.BindHost != "" && c.Server.BindHost != "0.0.0.0" {
		return c.Server.BindHost
	}
	return DetectOutboundIP()
}
