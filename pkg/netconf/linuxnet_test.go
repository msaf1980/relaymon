package netconf

import (
	"net"
	"strings"
	"testing"
)

func TestIPMaskEqual(t *testing.T) {
	tests := []struct {
		name  string
		a     net.IPMask
		b     net.IPMask
		equal bool
	}{
		{
			"255.255.255.0 = /24",
			net.IPv4Mask(255, 255, 255, 0),
			net.CIDRMask(24, 32),
			true,
		},
		{
			"255.255.255.0 = /25",
			net.IPv4Mask(255, 255, 255, 0),
			net.CIDRMask(25, 32),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IPMaskEqual(tt.a, tt.b); got != tt.equal {
				t.Errorf("IPMaskEqual() = %v, want %v", got, tt.equal)
			}
		})
	}
}

func Test_ipExec(t *testing.T) {
	iface := "lo"
	scope := "global"

	tests := []struct {
		action  string
		addr    string
		wantCmd string
	}{
		{"add", "192.168.151.11/24", "ip addr add dev lo 192.168.151.11/24 scope global"},
		{"del", "192.168.151.11/24", "ip addr del dev lo 192.168.151.11/24 scope global"},
	}
	for _, tt := range tests {
		t.Run(tt.action+" "+tt.addr, func(t *testing.T) {
			_, _, args := ipExec(iface, tt.addr, scope, (tt.action == "add"))
			cmd := strings.Join(args, " ")
			if cmd != tt.wantCmd {
				t.Errorf("ipExec() got command '%s', want '%s'", cmd, tt.wantCmd)
			}
		})
	}
}
