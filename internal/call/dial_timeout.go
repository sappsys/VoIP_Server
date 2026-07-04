package call

import "time"

// DialTimeout returns the outbound INVITE timeout for a single dial attempt.
func DialTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

// RingTimeout returns how long the caller hears ringback before busy.
func RingTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
