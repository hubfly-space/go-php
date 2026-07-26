package policy

import (
	"net"
	"sync"
)

// NetworkPolicy controls outbound network access.
type NetworkPolicy struct {
	mu           sync.RWMutex
	allowList    []net.IP // nil = allow all
	denyPrivate  bool
	denyLoopback bool
	denyRanges   []*net.IPNet
}

// NewNetworkPolicy creates a network policy.
func NewNetworkPolicy() *NetworkPolicy {
	return &NetworkPolicy{
		denyPrivate: true,
	}
}

// SetAllowList sets an IP allowlist. nil means allow all.
func (np *NetworkPolicy) SetAllowList(ips []net.IP) {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.allowList = ips
}

// SetDenyPrivate controls whether private IPs are blocked.
func (np *NetworkPolicy) SetDenyPrivate(deny bool) {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.denyPrivate = deny
}

// AddDeniedRange adds a CIDR range to deny.
func (np *NetworkPolicy) AddDeniedRange(cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	np.mu.Lock()
	defer np.mu.Unlock()
	np.denyRanges = append(np.denyRanges, network)
	return nil
}

// Check checks if a destination IP is allowed.
func (np *NetworkPolicy) Check(destIP net.IP) (allowed bool, reason string) {
	np.mu.RLock()
	defer np.mu.RUnlock()

	// Allowlist check.
	if np.allowList != nil {
		allowed := false
		for _, ip := range np.allowList {
			if ip.Equal(destIP) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, "not in allowlist"
		}
	}

	// Loopback check.
	if np.denyLoopback && destIP.IsLoopback() {
		return false, "loopback blocked"
	}

	// Private range check.
	if np.denyPrivate && isPrivateIP(destIP) {
		return false, "private IP blocked"
	}

	// Denied ranges check.
	for _, network := range np.denyRanges {
		if network.Contains(destIP) {
			return false, "IP in denied range"
		}
	}

	return true, ""
}

// IsPrivateIP checks if an IP is in a private range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// DNSRebindingDetector detects potential DNS rebinding attacks.
type DNSRebindingDetector struct {
	mu           sync.RWMutex
	resolvedIPs  map[string][]net.IP // hostname -> resolved IPs
	allowedHosts map[string]bool
}

// NewDNSRebindingDetector creates a DNS rebinding detector.
func NewDNSRebindingDetector() *DNSRebindingDetector {
	return &DNSRebindingDetector{
		resolvedIPs:  make(map[string][]net.IP),
		allowedHosts: make(map[string]bool),
	}
}

// RecordResolution records a DNS resolution.
func (d *DNSRebindingDetector) RecordResolution(hostname string, ips []net.IP) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.resolvedIPs[hostname] = ips
}

// CheckRebinding checks if a hostname has changed IPs (potential rebinding).
func (d *DNSRebindingDetector) CheckRebinding(hostname string, currentIP net.IP) (safe bool, reason string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	previous, ok := d.resolvedIPs[hostname]
	if !ok {
		return true, "" // first resolution
	}

	for _, prevIP := range previous {
		if prevIP.Equal(currentIP) {
			return true, ""
		}
	}

	return false, "IP changed since last resolution (potential DNS rebinding)"
}

// IsAllowedHost checks if a host is in the allow list.
func (d *DNSRebindingDetector) IsAllowedHost(hostname string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.allowedHosts[hostname]
}

// AllowHost adds a host to the allow list.
func (d *DNSRebindingDetector) AllowHost(hostname string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.allowedHosts[hostname] = true
}
