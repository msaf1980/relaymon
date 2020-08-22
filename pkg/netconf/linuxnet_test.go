package netconf

import (
	"net"
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
