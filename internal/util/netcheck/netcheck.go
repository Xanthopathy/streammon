package netcheck

import (
	"net"
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

var hostIndex = 0

// CheckInternetConnection attempts to connect to reliable public DNS servers to verify internet access.
// It rotates through multiple hosts for robustness and uses a short timeout to fail fast.
// Returns true only if at least one host can be reached.
func CheckInternetConnection() bool {
	// Try the next host in rotation
	host := reliableHosts[hostIndex%len(reliableHosts)]
	hostIndex++

	conn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err == nil {
		conn.Close()
		return true
	}

	// If primary check fails, try the next host as fallback
	for i := 1; i < len(reliableHosts); i++ {
		host = reliableHosts[(hostIndex+i)%len(reliableHosts)]
		conn, err := net.DialTimeout("tcp", host, 3*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
	}

	return false
}
