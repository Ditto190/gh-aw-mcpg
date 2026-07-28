package util

import "net"

// ClientAddr returns a client-friendly address from a listener address.
// When the host is a wildcard (empty, 0.0.0.0, ::, or [::]), it substitutes
// "localhost" so the printed address is usable from a client.
// Use this when a resolvable hostname is required (e.g. for GH_HOST with the gh CLI).
// For gateway output URLs where a numeric loopback address is preferred, use ClientHost.
func ClientAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return net.JoinHostPort("localhost", port)
	}
	return addr
}

// ClientHost returns a client-friendly hostname from a listener host string.
// When the host is a wildcard (empty, 0.0.0.0, ::, or [::]), it returns
// "127.0.0.1" so the address is usable in client-facing URLs.
// Use ClientAddr when you need to operate on a full host:port string.
func ClientHost(host string) string {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	}
	return host
}
