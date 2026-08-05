package observability

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// TrustedProxyConfig defines trusted proxy settings (§11.7).
type TrustedProxyConfig struct {
	TrustedNets []*net.IPNet
	MaxChain    int
}

// NewTrustedProxyConfig parses IP and CIDR strings into a TrustedProxyConfig.
func NewTrustedProxyConfig(trustedIPs []string, maxChain int) (*TrustedProxyConfig, error) {
	if maxChain <= 0 {
		maxChain = 5
	}

	var nets []*net.IPNet
	for _, raw := range trustedIPs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			ip := net.ParseIP(raw)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted IP: %s", raw)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			raw = fmt.Sprintf("%s/%d", raw, bits)
		}
		_, ipNet, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, fmt.Errorf("parse CIDR %s: %w", raw, err)
		}
		nets = append(nets, ipNet)
	}

	return &TrustedProxyConfig{
		TrustedNets: nets,
		MaxChain:    maxChain,
	}, nil
}

// IsTrusted returns true if peerIP is in the trusted networks list.
func (c *TrustedProxyConfig) IsTrusted(peerIP string) bool {
	if len(c.TrustedNets) == 0 {
		return false
	}
	ip := net.ParseIP(peerIP)
	if ip == nil {
		host, _, err := net.SplitHostPort(peerIP)
		if err == nil {
			ip = net.ParseIP(host)
		}
	}
	if ip == nil {
		return false
	}

	for _, n := range c.TrustedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP extracts the canonical client IP from request, taking trusted proxies into account.
func (c *TrustedProxyConfig) ClientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = host
	}

	if !c.IsTrusted(peer) {
		return peer
	}

	// Read X-Forwarded-For
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer
	}

	rawIPs := strings.Split(xff, ",")
	var validIPs []string
	for _, raw := range rawIPs {
		ipStr := strings.TrimSpace(raw)
		if ip := net.ParseIP(ipStr); ip != nil {
			validIPs = append(validIPs, ipStr)
		}
	}

	if len(validIPs) == 0 {
		return peer
	}

	// Limit chain length
	if len(validIPs) > c.MaxChain {
		validIPs = validIPs[len(validIPs)-c.MaxChain:]
	}

	// Walk backwards from immediate peer to find first untrusted client IP
	for i := len(validIPs) - 1; i >= 0; i-- {
		if !c.IsTrusted(validIPs[i]) {
			return validIPs[i]
		}
	}

	return validIPs[0]
}
