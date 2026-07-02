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
	if c.Server.BindHost != "" && c.Server.BindHost != "0.0.0.0" && c.Server.BindHost != "::" {
		return c.Server.BindHost
	}
	return DetectOutboundIP()
}

// SIPBindHost returns the local IP diago/sipgo should bind for SIP and RTP.
// When bind_host is 0.0.0.0, diago auto-picks an interface (often docker0) for
// outbound calls, which breaks B2BUA INVITEs. Prefer external_host in that case.
func (c *Config) SIPBindHost() string {
	bind := c.Server.BindHost
	if bind == "" {
		bind = "0.0.0.0"
	}
	if bind == "0.0.0.0" || bind == "::" {
		if c.Server.ExternalHost != "" {
			return c.Server.ExternalHost
		}
	}
	return bind
}
