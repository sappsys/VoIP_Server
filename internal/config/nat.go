package config

import "fmt"

// NATConfig controls STUN and optional extra SIP listen ports for NAT traversal.
type NATConfig struct {
	// STUNEnabled runs a RFC 5389 STUN server (UDP) so clients can discover reflexive addresses.
	STUNEnabled bool `toml:"stun_enabled"`
	// STUNBindHost is the local address for STUN (default: server.bind_host).
	STUNBindHost string `toml:"stun_bind_host"`
	// STUNPort is the UDP port for STUN (default 3478).
	STUNPort int `toml:"stun_port"`

	// SIPProxyEnabled adds a second SIP listen port for NAT clients / firewall pinholes.
	SIPProxyEnabled bool `toml:"sip_proxy_enabled"`
	// SIPProxyPort is the extra SIP port (default 5061).
	SIPProxyPort int `toml:"sip_proxy_port"`
}

func setNATDefaults(cfg *Config) {
	if cfg.NAT.STUNPort == 0 {
		cfg.NAT.STUNPort = 3478
	}
	if cfg.NAT.SIPProxyPort == 0 {
		cfg.NAT.SIPProxyPort = 5061
	}
}

// STUNBindHost returns the address the STUN server should bind to.
func (c *Config) STUNBindHost() string {
	if c.NAT.STUNBindHost != "" {
		return c.NAT.STUNBindHost
	}
	if c.Server.BindHost != "" {
		return c.Server.BindHost
	}
	return "0.0.0.0"
}

func (c *Config) validateNAT() error {
	if c.NAT.STUNEnabled {
		if err := validatePort("nat.stun_port", c.NAT.STUNPort); err != nil {
			return err
		}
		if c.NAT.STUNPort == c.Server.BindPort {
			return fmt.Errorf("nat.stun_port must differ from server.bind_port (%d)", c.Server.BindPort)
		}
	}
	if c.NAT.SIPProxyEnabled {
		if err := validatePort("nat.sip_proxy_port", c.NAT.SIPProxyPort); err != nil {
			return err
		}
		if c.NAT.SIPProxyPort == c.Server.BindPort {
			return fmt.Errorf("nat.sip_proxy_port must differ from server.bind_port (%d)", c.Server.BindPort)
		}
		if c.NAT.STUNEnabled && c.NAT.SIPProxyPort == c.NAT.STUNPort {
			return fmt.Errorf("nat.sip_proxy_port must differ from nat.stun_port (%d)", c.NAT.STUNPort)
		}
	}
	return nil
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}
	return nil
}
