package media

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"
)

const rtpBuf = 1500

// Relay forwards RTP between two local UDP sockets and their remote peers.
type Relay struct {
	Log *slog.Logger
}

func (r *Relay) Start(ctx context.Context, connA, connB *net.UDPConn, remoteA, remoteB *net.UDPAddr) {
	var wg sync.WaitGroup
	pump := func(conn *net.UDPConn, dest *net.UDPAddr) {
		defer wg.Done()
		buf := make([]byte, rtpBuf)
		target := dest
		for {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			if from != nil && dest != nil && from.IP.Equal(dest.IP) {
				target = from
			}
			if target == nil {
				target = dest
			}
			if target == nil {
				continue
			}
			_, _ = conn.WriteToUDP(buf[:n], target)
		}
	}
	wg.Add(2)
	go pump(connA, remoteB)
	go pump(connB, remoteA)
	go func() {
		<-ctx.Done()
		_ = connA.Close()
		_ = connB.Close()
		wg.Wait()
	}()
}

// AllocateVideoPorts returns two UDP listeners on ephemeral ports.
func AllocateVideoPorts() (portA, portB int, connA, connB *net.UDPConn, err error) {
	connA, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return 0, 0, nil, nil, err
	}
	connB, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		_ = connA.Close()
		return 0, 0, nil, nil, err
	}
	return connA.LocalAddr().(*net.UDPAddr).Port, connB.LocalAddr().(*net.UDPAddr).Port, connA, connB, nil
}
