package netconf

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"time"
)

func ipExec(iface string, addr string, scope string, add bool) (string, error) {
	var ipArgs []string
	if add {
		ipArgs = []string{"ip", "addr", "add", "dev", iface, addr, "scope", scope}
	} else {
		ipArgs = []string{"ip", "addr", "del", "dev", iface, addr, "scope", scope}
	}

	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ip")
	cmd.Args = ipArgs
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("command timeout")
	} else if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			err = fmt.Errorf("command exit with %d", exitErr.ExitCode())
		} else {
			err = fmt.Errorf("command execute error with %s", err.Error())
		}
	}
	return string(out), err
}

// IPMaskEqual compare net.IPMask
func IPMaskEqual(a net.IPMask, b net.IPMask) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// FindIPNet search IPNet in slice
func FindIPNet(n *net.IPNet, ns []net.Addr) bool {
	for i := range ns {
		if ipnet, ok := ns[i].(*net.IPNet); ok {
			if ipnet.IP.Equal(n.IP) && IPMaskEqual(ipnet.Mask, n.Mask) {
				return true
			}
		}
	}
	return false
}

// IfaceAddrs return interface addresses
func IfaceAddrs(iface string) ([]net.Addr, error) {
	i, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}

	addrs, err := i.Addrs()
	if err != nil {
		return nil, err
	}

	return addrs, nil
}

// IfaceAddrAdd configure ip addresses on interface (in ip/net format)
func IfaceAddrAdd(iface string, a []*net.IPNet) []error {
	errs := make([]error, 0)
	addrs, err := IfaceAddrs(iface)
	if err != nil {
		errs = append(errs, err)
		return errs
	}
	for _, addr := range a {
		if !FindIPNet(addr, addrs) {
			//fmt.Printf("%s\n", addr.String())
			out, err := ipExec(iface, addr.String(), "global", true)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s ip add %s with %s:\n%s", iface, addr.String(), err.Error(), out))
			}
		}
	}
	return errs
}

// IfaceAddrDel remove ip addresses on interface
func IfaceAddrDel(iface string, a []*net.IPNet) []error {
	errs := make([]error, 0)
	addrs, err := IfaceAddrs(iface)
	if err != nil {
		errs = append(errs, err)
		return errs
	}
	for _, addr := range a {
		if FindIPNet(addr, addrs) {
			//fmt.Printf("%s\n", addr.IP.String())
			out, err := ipExec(iface, addr.String(), "global", false)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s ip del %s with %s:\n%s", iface, addr.IP.String(), err.Error(), out))
			}
		}
	}
	return errs
}
