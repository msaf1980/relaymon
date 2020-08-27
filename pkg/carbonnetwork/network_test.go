package carbonnetwork

import (
	"net"
	"testing"
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
	t       *testing.T
}

func newTCPServer(t *testing.T, address string, failure FailureType) server {
	s := &tcpServer{failure: failure, t: t}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		s.t.Fatalf("listen failed: %s", err.Error())
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
					s.t.Fatalf("Error accepting connection: %v\n", err)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFarm := newServerFarm(t)
			testFarm.AppendTCPServers(tt.cluster.Endpoints, tt.serversFailure)
			defer testFarm.Stop()

			got, gotErr := tt.cluster.Check()
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
