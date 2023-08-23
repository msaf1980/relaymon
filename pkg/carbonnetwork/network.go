package carbonnetwork

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/msaf1980/relaymon/pkg/checker"
	"github.com/msaf1980/relaymon/pkg/neterror"
	"github.com/rs/zerolog/log"
)

// Cluster describe group of network endpoints
type Cluster struct {
	Name        string
	Endpoints   []string
	TestMetrics []string
	testPrefix  string
	Errors      []error
	timeout     time.Duration
	Required    bool
}

type check struct {
	N   int
	Err error
}

// NewCluster alloc new cluster instance
func NewCluster(name string, required bool, testPrefix string, timeout time.Duration) *Cluster {
	return &Cluster{Name: name, Required: required, testPrefix: testPrefix, timeout: timeout}
}

// Append append cluster endpoint
func (c *Cluster) Append(endpoint string) *Cluster {
	c.Endpoints = append(c.Endpoints, endpoint)
	c.Errors = append(c.Errors, nil)
	testMetric := fmt.Sprintf("%s.test.network.carbon.%s.%s", c.testPrefix, checker.Strip(c.Name), checker.Strip(endpoint))
	c.TestMetrics = append(c.TestMetrics, testMetric)
	return c
}

// Check cluster status (success, errors)
func (c *Cluster) Check(ctx context.Context, timestamp int64) (bool, []error) {
	out := make(chan check, len(c.Endpoints))
	defer close(out)

	for i := range c.Endpoints {
		go func(out chan check, n int) {
			log.Trace().Str("action", "check").Str("network_checker", "carbon").Int("n", n).Str("endpoint", c.Endpoints[n]).Msg("next check iteration")

			ctxTout, cancel := context.WithTimeout(ctx, c.timeout)
			defer cancel()

			var d net.Dialer

			conn, err := d.DialContext(ctxTout, "tcp", c.Endpoints[n])
			if err != nil {
				out <- check{n, neterror.NewNetError(err)}
			} else {

				writeDone := make(chan struct{})
				defer close(writeDone)
				// setup the cancellation to abort writes in process
				go func() {
					select {
					case <-ctx.Done():
						conn.Close()
						// Close() can be used if this isn't necessarily a TCP connection
					case <-writeDone:
					}
				}()

				send := []string{c.TestMetrics[n], " 1 ", strconv.FormatInt(timestamp, 10), "\n"}
				for j := range send {

					log.Trace().Str("action", "check").Str("network_checker", "carbon").Int("n", n).Str("endpoint", c.Endpoints[n]).Int("send", j).Msg("write")

					_, err = conn.Write([]byte(send[j]))
					if err != nil {
						conn.Close()
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err == nil {
					err = conn.Close()
				} else {
					conn.Close()
				}
				out <- check{n, neterror.NewNetError(err)}

				log.Trace().Str("action", "check").Str("network_checker", "carbon").Int("n", n).Str("endpoint", c.Endpoints[n]).Msg("next check iteration")
			}

			log.Trace().Str("action", "check").Str("network_checker", "carbon").Int("n", n).Str("endpoint", c.Endpoints[n]).Msg("end check iteration")

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
	clusters []*Cluster

	// check results
	failed  int
	success int
	checked int

	// check thresholds
	metrics    []checker.Metric
	failCount  int
	checkCount int
	resetCount int

	notify bool
}

// NewNetworkChecker return new systemd service instance
func NewNetworkChecker(name string, clusters []*Cluster, timeout time.Duration,
	failCount int, checkCount int, resetCount int) *NetworkChecker {

	n := 0
	for i := range clusters {
		n += len(clusters[i].Endpoints)
	}

	network := &NetworkChecker{
		name:       name,
		clusters:   clusters,
		failCount:  failCount,
		checkCount: checkCount,
		resetCount: resetCount,
		metrics:    make([]checker.Metric, n),
	}
	n = 0
	for i := range clusters {
		for j := range clusters[i].Endpoints {
			network.metrics[n].Name = "network.carbon." + checker.Strip(clusters[i].Name) + "." + checker.Strip(clusters[i].Endpoints[j])
			network.metrics[n].Value = strconv.Itoa(int(checker.CollectingState))
			n++
		}
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
func (n *NetworkChecker) Status(ctx context.Context, timestamp int64) (checker.State, []string) {
	successCheck := true
	events := make([]string, 0)

	failed := 0
	k := 0
	for i := range n.clusters {
		clusterStatus, clusterErrs := n.clusters[i].Check(ctx, timestamp)
		if !clusterStatus {
			failed++
			if n.clusters[i].Required {
				successCheck = false
			}
		}
		for j := range clusterErrs {
			if clusterErrs[j] != nil {
				errMetric := strconv.Itoa(int(checker.ErrorState))
				if n.metrics[k].Value != errMetric {
					n.metrics[k].Value = errMetric
				}
				if checker.ErrorChanged(n.clusters[i].Errors[j], clusterErrs[j]) {
					events = append(events, fmt.Sprintf("endpoint %s %s", n.clusters[i].Endpoints[j], clusterErrs[j].Error()))
				}
			} else {
				successMetric := strconv.Itoa(int(checker.SuccessState))
				if n.metrics[k].Value != successMetric {
					n.metrics[k].Value = successMetric
				}
				if checker.ErrorChanged(n.clusters[i].Errors[j], clusterErrs[j]) {
					events = append(events, fmt.Sprintf("endpoint %s up", n.clusters[i].Endpoints[j]))
				}
			}
			n.clusters[i].Errors[j] = clusterErrs[j]
			k++
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
		return checker.CollectingState, events
	} else if n.failed > 0 {
		if n.failed >= n.failCount {
			return checker.ErrorState, events
		}
		return checker.WarnState, events
	}
	return checker.SuccessState, events
}

// Metrics get metric for status check
func (n *NetworkChecker) Metrics() []checker.Metric {
	return n.metrics
}
