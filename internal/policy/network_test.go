package policy

import (
	"net"
	"testing"
)

func TestNetworkPolicyDefault(t *testing.T) {
	np := NewNetworkPolicy()

	// Public IP should be allowed.
	publicIP := net.ParseIP("8.8.8.8")
	allowed, reason := np.Check(publicIP)
	if !allowed {
		t.Errorf("public IP should be allowed, reason: %s", reason)
	}

	// Private IP should be denied by default.
	privateIP := net.ParseIP("10.0.0.1")
	allowed, reason = np.Check(privateIP)
	if allowed {
		t.Error("private IP should be denied by default")
	}
	_ = reason
}

func TestNetworkPolicyAllowPrivate(t *testing.T) {
	np := NewNetworkPolicy()
	np.SetDenyPrivate(false)

	privateIP := net.ParseIP("192.168.1.1")
	allowed, _ := np.Check(privateIP)
	if !allowed {
		t.Error("private IP should be allowed when deny_private=false")
	}
}

func TestNetworkPolicyAllowList(t *testing.T) {
	np := NewNetworkPolicy()
	np.SetDenyPrivate(false)
	np.SetAllowList([]net.IP{
		net.ParseIP("1.2.3.4"),
	})

	tests := []struct {
		ip      string
		allowed bool
	}{
		{"1.2.3.4", true},
		{"5.6.7.8", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		allowed, _ := np.Check(ip)
		if allowed != tt.allowed {
			t.Errorf("IP %s: allowed=%v, want %v", tt.ip, allowed, tt.allowed)
		}
	}
}

func TestNetworkPolicyDenyRange(t *testing.T) {
	np := NewNetworkPolicy()
	np.SetDenyPrivate(false)
	np.AddDeniedRange("192.168.1.0/24")

	deniedIP := net.ParseIP("192.168.1.100")
	allowed, reason := np.Check(deniedIP)
	if allowed {
		t.Error("IP in denied range should be blocked")
	}
	if reason != "IP in denied range" {
		t.Errorf("reason = %q", reason)
	}

	safeIP := net.ParseIP("192.168.2.1")
	allowed, _ = np.Check(safeIP)
	if !allowed {
		t.Error("IP outside denied range should be allowed")
	}
}

func TestNetworkPolicyInvalidRange(t *testing.T) {
	np := NewNetworkPolicy()
	err := np.AddDeniedRange("not-a-cidr")
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if got := isPrivateIP(ip); got != tt.private {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
		}
	}
}

func TestDNSRebindingDetector(t *testing.T) {
	d := NewDNSRebindingDetector()

	host := "example.com"
	ip1 := net.ParseIP("1.2.3.4")
	ip2 := net.ParseIP("5.6.7.8")

	// First resolution — always safe.
	d.RecordResolution(host, []net.IP{ip1})
	safe, _ := d.CheckRebinding(host, ip1)
	if !safe {
		t.Error("first resolution should be safe")
	}

	// Same IP — safe.
	safe, _ = d.CheckRebinding(host, ip1)
	if !safe {
		t.Error("same IP should be safe")
	}

	// Different IP — rebinding detected.
	safe, reason := d.CheckRebinding(host, ip2)
	if safe {
		t.Error("different IP should trigger rebinding detection")
	}
	if reason == "" {
		t.Error("expected reason for rebinding")
	}
}

func TestDNSRebindingDetectorAllowHost(t *testing.T) {
	d := NewDNSRebindingDetector()

	if d.IsAllowedHost("example.com") {
		t.Error("should not be allowed by default")
	}

	d.AllowHost("example.com")
	if !d.IsAllowedHost("example.com") {
		t.Error("should be allowed after AllowHost")
	}
}
