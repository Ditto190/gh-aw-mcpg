package util

import "net"

// ClientAddr returns a client-friendly address from a listener address.
// When the host is a wildcard (0.0.0.0, ::, or empty), it substitutes
// "localhost" so the printed address is usable from a client.
//
// Note: output.go uses "127.0.0.1" for the same wildcard substitution in
// the gateway config output, while this function uses "localhost" because
// GH_HOST must be a resolvable hostname for the gh CLI.
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
