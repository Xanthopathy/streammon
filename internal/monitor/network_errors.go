package monitor

import (
	"context"
	"strings"
	"time"
)

// NetworkError represents an error caused by internet connectivity issues.
// These errors should NOT count toward backoff timers, as the connection monitor
// will pause operations anyway when offline.
type NetworkError struct {
	Err error
}

// Error implements the error interface for NetworkError.
func (ne *NetworkError) Error() string {
	return ne.Err.Error()
}

// Unwrap returns the underlying error for error chain compatibility.
func (ne *NetworkError) Unwrap() error {
	return ne.Err
}

// IsNetworkError checks if an error is a NetworkError.
func IsNetworkError(err error) bool {
	_, ok := err.(*NetworkError)
	return ok
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isConnectivityError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	networkErrPatterns := []string{
		// DNS resolution failures
		"no such host", "lookup", "temporary failure in name resolution",
		// Connection establishment failures
		"dial tcp", "connection refused", "connection reset",
		// Active connection failures (e.g., socket reads/writes during existing connection)
		"read tcp", "write tcp", "wsarecv", "wsasend",
		// Timeouts and unreachable network states
		"context canceled", "context deadline exceeded", "client.timeout exceeded", "i/o timeout", "timeout",
		"network is unreachable", "network is down", "host is down",
	}
	for _, pattern := range networkErrPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}
