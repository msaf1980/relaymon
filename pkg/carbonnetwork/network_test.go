package carbonnetwork

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/msaf1980/relaymon/pkg/checker"
)

type FailureType int8

const (
	noFailure FailureType = iota
	noListen
	noReadWithClose
)

type server interface {
	Address() string
	Stop()
}

type tcpServer struct {
	address string
	failure FailureType
	ln      net.Listener
	conns   chan net.Conn
	stop    chan bool
	running bool
}

func newTCPServer(t *testing.T, address string, failure FailureType) server {
	s := &tcpServer{failure: failure}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listen failed: %s", err.Error())
	}
	s.address = ln.Addr().String()
	if failure == noListen {
		ln.Close()
	} else {
		s.stop = make(chan bool)
		s.ln = ln
		s.accept()
	}

	return s
}

func (s *tcpServer) Address() string {
	return s.address
}

func (s *tcpServer) accept() {
	s.conns = make(chan net.Conn)

	// Handle connections
	s.running = true
	go func() {
		for s.running {
			select {
			case conn := <-s.conns:
				// Handle incomming connection
				go handleTCPConnection(conn)
			case <-s.stop:
				s.running = false
				s.ln.Close()
				break
			}
		}
	}()

	// Nadle accept
	go func() {
		for s.running {
			conn, err := s.ln.Accept()
			if err != nil {
				if s.running {
					fmt.Fprintf(os.Stderr, "Error accepting connection: %s\n", err.Error())
				} else {
					break
				}
			}
			if s.failure == noReadWithClose {
				conn.Close()
			} else {
				s.conns <- conn
			}
		}
	}()
}

func (s *tcpServer) Stop() {
	if s.failure != noListen {
		s.stop <- true
	}
}

func handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	recvBuf := make([]byte, 1024)
	for {
		if _, err := conn.Read(recvBuf); err != nil {
			break
		}
	}
}

type serversFarm struct {
	Servers []server
	t       *testing.T
}

func newServerFarm(t *testing.T) *serversFarm {
	f := &serversFarm{t: t}
	return f
}

func (f *serversFarm) AppendTCPServers(addrs []string, failure []FailureType) *serversFarm {
	n := len(f.Servers)
	for i := range addrs {
		s := newTCPServer(f.t, addrs[i], failure[i])
		f.Servers = append(f.Servers, s)
		addrs[i] = f.Servers[n+i].Address()
	}
	return f
}

func (f *serversFarm) Stop() {
	for i := range f.Servers {
		f.Servers[i].Stop()
	}
}

func TestCluster_Check(t *testing.T) {
	tests := []struct {
		name           string
		cluster        *Cluster
		serversFailure []FailureType
		want           bool
		wantErr        []bool
	}{
		{
			name: "All must failed",
			cluster: NewCluster("all_failed", false).
				Append("127.0.0.1:0").Append("127.0.0.1:0"),
			serversFailure: []FailureType{noListen, noReadWithClose},
			want:           false,
			wantErr:        []bool{true, true},
		},
		{
			name: "One must successed",
			cluster: NewCluster("one_successed", false).
				Append("127.0.0.1:0").Append("127.0.0.1:0").Append("127.0.0.1:0"),
			serversFailure: []FailureType{noListen, noReadWithClose, noFailure},
			want:           true,
			wantErr:        []bool{true, true, false},
		},
		{
			name: "All successed",
			cluster: NewCluster("all_successed", false).
				Append("127.0.0.1:0").Append("127.0.0.1:0").Append("127.0.0.1:0"),
			serversFailure: []FailureType{noFailure, noFailure, noFailure},
			want:           true,
			wantErr:        []bool{false, false, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFarm := newServerFarm(t)
			testFarm.AppendTCPServers(tt.cluster.Endpoints, tt.serversFailure)
			defer testFarm.Stop()

			got, gotErr := tt.cluster.Check(0)
			if got != tt.want {
				t.Errorf("%s Cluster.Check() got = %v, want %v", tt.cluster.Name, got, tt.want)
			}
			for i := range gotErr {
				if (gotErr[i] != nil) != tt.wantErr[i] {
					if gotErr[i] == nil {
						t.Errorf("%s Cluster.Check() got success for %s (%d), want failed", tt.cluster.Name,
							tt.cluster.Endpoints[i], i,
						)
					} else {
						t.Errorf("%s Cluster.Check() got failed for %s (%s) for %s (%d), want success", tt.cluster.Name,
							tt.cluster.Endpoints[i], gotErr[i].Error(), tt.cluster.Name, i,
						)
					}
				}
			}
		})
	}
}

func TestNetworkChecker_Status(t *testing.T) {
	failCount := 2
	checkCount := 3
	resetCount := 2

	tests := []struct {
		name        string
		clusters    []*Cluster
		failures    [][]FailureType
		want        checker.State
		wantMetrics []string
	}{
		{
			name: "all_failed",
			clusters: []*Cluster{
				NewCluster("all_failed", false).
					Append("127.0.0.1:0").Append("127.0.0.1:0"),
			},
			failures: [][]FailureType{
				[]FailureType{noListen, noReadWithClose},
			},
			want: checker.ErrorState,
		},
		{
			name: "one_success",
			clusters: []*Cluster{
				NewCluster("one_success", false).
					Append("127.0.0.1:0").Append("127.0.0.1:0"),
			},
			failures: [][]FailureType{
				[]FailureType{noListen, noFailure},
			},
			want: checker.SuccessState,
		},
		{
			name: "required_failed",
			clusters: []*Cluster{
				NewCluster("one_success", false).
					Append("127.0.0.1:0").Append("127.0.0.1:0"),
				NewCluster("all_failed", true).
					Append("127.0.0.1:0").Append("127.0.0.1:0"),
			},
			failures: [][]FailureType{
				[]FailureType{noListen, noFailure},
				[]FailureType{noListen, noReadWithClose},
			},
			want: checker.ErrorState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFarm := newServerFarm(t)
			for i := range tt.failures {
				testFarm.AppendTCPServers(tt.clusters[i].Endpoints, tt.failures[i])
			}
			defer testFarm.Stop()

			c := NewNetworkChecker(tt.name, tt.clusters, time.Second, failCount, checkCount, resetCount)
			for k := range tt.clusters {
				for e := range tt.clusters[k].Endpoints {
					tt.wantMetrics = append(tt.wantMetrics,
						fmt.Sprintf("network.carbon.%s.%s", checker.Strip(tt.clusters[k].Name), checker.Strip(tt.clusters[k].Endpoints[e])),
					)
				}
			}
			for i := 0; i < checkCount+1; i++ {
				got, _ := c.Status(0)
				want := checker.CollectingState
				if i >= checkCount-1 {
					want = tt.want
				}
				if got != want {
					t.Errorf("Step %d ServiceChecker.Status() got = %v, want %v", i, got, want)
				}
				metrics := c.Metrics()
				n := 0
				for k := range tt.clusters {
					for e := range tt.clusters[k].Endpoints {
						want := checker.ErrorState
						if tt.failures[k][e] == noFailure {
							want = checker.SuccessState
						}
						if metrics[n].Name != tt.wantMetrics[n] {
							t.Errorf("Step %d ServiceChecker.Metrics()[%d] name got = '%s', want '%s'", i, n,
								metrics[n].Name, tt.wantMetrics[n],
							)
						}
						if metrics[n].Value != strconv.Itoa(int(want)) {
							t.Errorf("Step %d ServiceChecker.Metrics()[%d] got = %v, want %v", i, n,
								metrics[n], checker.Metric{Name: tt.wantMetrics[n], Value: strconv.Itoa(int(want))},
							)
						}
						n++
					}
				}
			}
		})
	}
}
