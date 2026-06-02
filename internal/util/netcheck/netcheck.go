package netcheck

import (
	"net"
	"sync/atomic"
	"time"
)

// reliableHosts is a list of reliable public DNS/HTTP servers to check for internet connectivity.
// These are rotated through to ensure robust connection detection.
var reliableHosts = []string{
	"1.1.1.1:53",        // Cloudflare DNS
	"8.8.8.8:53",        // Google DNS
	"9.9.9.9:53",        // Quad9 DNS
	"208.67.222.222:53", // OpenDNS
}

var hostIndex atomic.Uint64

// CheckInternetConnection attempts to connect to reliable public DNS servers to verify internet access.
// It rotates through multiple hosts for robustness and uses a short timeout to fail fast.
// Returns true only if at least one host can be reached.
func CheckInternetConnection() bool {
	start := hostIndex.Add(1) - 1
	hostCount := uint64(len(reliableHosts))

	for i := uint64(0); i < hostCount; i++ {
		host := reliableHosts[(start+i)%hostCount]
		conn, err := net.DialTimeout("tcp", host, 3*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
	}

	return false
}
