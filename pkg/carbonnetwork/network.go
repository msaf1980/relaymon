package carbonnetwork

import (
	"fmt"
	"math"
	"net"
	"time"

	"github.com/msaf1980/relaymon/pkg/checker"
	"github.com/msaf1980/relaymon/pkg/neterror"
)

// Cluster describe group of network endpoints
type Cluster struct {
	Name      string
	Endpoints []string
	Errors    []error
	timeout   time.Duration
	Required  bool
}

type check struct {
	N   int
	Err error
}

// Append append cluster endpoint
func (c *Cluster) Append(endpoint string) {
	c.Endpoints = append(c.Endpoints, endpoint)
	c.Errors = append(c.Errors, nil)
}

// Check cluster status (success, errors)
func (c *Cluster) Check() (bool, []error) {
	out := make(chan check, len(c.Endpoints))
	defer close(out)

	for i := range c.Endpoints {
		go func(out chan check, n int) {
			conn, err := net.DialTimeout("tcp", c.Endpoints[n], c.timeout)
			if err != nil {
				out <- check{n, neterror.NewNetError(err)}
			} else {
				_ = conn.SetReadDeadline(time.Now().Add(c.timeout))
				_, err = conn.Write([]byte("test"))
				conn.Close()
				out <- check{n, neterror.NewNetError(err)}
			}
		}(out, i)
	}

	checks := make([]error, len(c.Endpoints))
	count := 0
	failed := 0
	for count < len(c.Endpoints) {
		n := <-out
		checks[n.N] = n.Err
		count++
		if n.Err != nil {
			failed++
		}
	}

	return failed < count, checks
}

// NetworkChecker check group of network endpoints with tcp connect and write test
type NetworkChecker struct {
	name     string
	clusters []Cluster

	// check results
	failed  int
	success int
	checked int

	// check thresholds
	failCount  int
	checkCount int
	resetCount int

	notify bool
}

// NewNetworkChecker return new systemd service instance
func NewNetworkChecker(name string, clusters []Cluster, timeout time.Duration,
	failCount int, checkCount int, resetCount int) *NetworkChecker {

	network := &NetworkChecker{
		name:       name,
		clusters:   clusters,
		failCount:  failCount,
		checkCount: checkCount,
		resetCount: resetCount,
	}
	return network
}

// SetNotify set relay and prefix for send metrics
func (n *NetworkChecker) SetNotify(notify bool) {
	n.notify = notify
}

// Name get check name
func (n *NetworkChecker) Name() string {
	return n.name
}

// Status get result of network status check
func (n *NetworkChecker) Status() (checker.State, []error) {
	successCheck := true
	errs := make([]error, 0, 10)

	failed := 0
	for i := range n.clusters {
		clusterStatus, clusterErrs := n.clusters[i].Check()
		if !clusterStatus {
			failed++
			if n.clusters[i].Required {
				successCheck = false
			}
		}
		for j := range clusterErrs {
			clusterErr := fmt.Errorf("endpoint %s %s", n.clusters[i].Endpoints[j], clusterErrs[j].Error())
			if n.clusters[i].Errors[j] != clusterErr {
				n.clusters[i].Errors[j] = clusterErr
				errs = append(errs, clusterErr)
			}
		}
	}
	if successCheck && failed == len(n.clusters) {
		successCheck = false
	}

	if n.checked < math.MaxInt32 {
		n.checked++
	}

	if successCheck {
		if n.success < math.MaxInt32 {
			n.success++
		}
		if n.failed > 0 && n.success >= n.resetCount {
			n.failed = 0
		}
	} else {
		if n.success > 0 {
			n.success = 0
		}
		if n.failed < math.MaxInt32 {
			n.failed++
		}
	}
	if n.checked < n.checkCount {
		return checker.CollectingState, errs
	} else if n.failed > 0 {
		if n.failed >= n.failCount {
			return checker.ErrorState, errs
		}
		return checker.WarnState, errs
	}
	return checker.SuccessState, errs
}
