package nat

import (
	"context"
	"log/slog"
	"net"
	"strconv"

	"github.com/pion/stun/v3"
)

// STUNServer answers STUN Binding requests with the client's reflexive transport address.
type STUNServer struct {
	Log *slog.Logger
}

func (s *STUNServer) Run(ctx context.Context, bindHost string, port int) error {
	if port <= 0 {
		port = 3478
	}
	addr := net.JoinHostPort(bindHost, strconv.Itoa(port))
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	defer pc.Close()

	if s.Log != nil {
		s.Log.Info("stun listening", "addr", addr)
	}

	buf := make([]byte, 1500)
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		n, remote, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		s.handlePacket(pc, buf[:n], remote)
	}
}

func (s *STUNServer) handlePacket(pc net.PacketConn, data []byte, remote net.Addr) {
	if !stun.IsMessage(data) {
		return
	}
	msg := &stun.Message{Raw: data}
	if err := msg.Decode(); err != nil {
		if s.Log != nil {
			s.Log.Debug("stun decode failed", "from", remote, "error", err)
		}
		return
	}
	if msg.Type != stun.BindingRequest {
		return
	}

	host, portStr, err := net.SplitHostPort(remote.String())
	if err != nil {
		if s.Log != nil {
			s.Log.Debug("stun remote addr parse failed", "from", remote, "error", err)
		}
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return
	}

	res, err := stun.Build(
		stun.NewTransactionIDSetter(msg.TransactionID),
		stun.BindingSuccess,
		&stun.XORMappedAddress{IP: ip, Port: port},
		stun.Fingerprint,
	)
	if err != nil {
		if s.Log != nil {
			s.Log.Debug("stun response build failed", "from", remote, "error", err)
		}
		return
	}
	if _, err := pc.WriteTo(res.Raw, remote); err != nil && s.Log != nil {
		s.Log.Debug("stun response send failed", "to", remote, "error", err)
		return
	}
	if s.Log != nil {
		s.Log.Debug("stun binding success", "client", remote, "mapped", net.JoinHostPort(ip.String(), portStr))
	}
}
