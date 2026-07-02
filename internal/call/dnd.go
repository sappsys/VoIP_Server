package call

import (
	"context"
	"time"

	"github.com/emiago/diago"
)

// RingCaller sends ringing to the caller without delivering the call to an endpoint.
func RingCaller(ctx context.Context, in *diago.DialogServerSession) error {
	in.Trying()
	if err := in.Ringing(); err != nil {
		return err
	}
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-in.Context().Done():
			return nil
		case <-ticker.C:
			_ = in.Ringing()
		}
	}
}
