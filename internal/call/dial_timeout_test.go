package call

import (
	"testing"
	"time"
)

func TestDialTimeoutDefault(t *testing.T) {
	if DialTimeout(0) != 15*time.Second {
		t.Fatalf("default=%v", DialTimeout(0))
	}
	if DialTimeout(15) != 15*time.Second {
		t.Fatalf("15s=%v", DialTimeout(15))
	}
}

func TestRingTimeoutDefault(t *testing.T) {
	if RingTimeout(0) != 30*time.Second {
		t.Fatalf("default=%v", RingTimeout(0))
	}
}
