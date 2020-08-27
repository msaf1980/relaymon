package systemd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/msaf1980/relaymon/pkg/checker"
	"github.com/msaf1980/relaymon/pkg/linuxproc"
)

func getServicePID(t *testing.T, service string) int64 {
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("/bin/systemctl status %s | grep 'Main PID:' | awk '{ print $3; }'", service))
	var stdOut bytes.Buffer
	cmd.Stdout = &stdOut
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Fatal error on get service pid: %s", err.Error())
		return -1
	}
	s := strings.Replace(stdOut.String(), "\n", "", 1)
	pid, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err.Error())
		return -1
	}
	return pid
}

// ServiceState return SystemdService
func getServiceForTest(t *testing.T, running bool) (string, *Service) {
	cmd := exec.Command("sh")
	if running {
		cmd.Args = []string{"sh", "-c", "/bin/systemctl -a | grep -w running | grep -w active | awk '{ print $1; }'  | grep '.service' | head -1"}
	} else {
		cmd.Args = []string{"sh", "-c", "/bin/systemctl -a | grep -w -v running | grep -w inactive | awk '{ print $1; }' | grep '.service' | head -1"}
	}
	var stdOut bytes.Buffer
	cmd.Stdout = &stdOut
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Fatal error on tail service: %s", err.Error())
		return "", nil
	}
	s := strings.Replace(stdOut.String(), "\n", "", 1)
	// if strings.HasSuffix(s, ".service") {
	// 	s = s[0 : len(s)-8]
	// }
	service := &Service{}
	if running {
		service.State = StartedState
		service.PID = getServicePID(t, s)
		proc, err := linuxproc.ProcInfo(service.PID)
		if err != nil {
			t.Fatalf("Fatal error on get service process: %s", err.Error())
			service.ProcName = s
		} else {
			service.ProcName = proc.ProcName
		}
	} else {
		service.State = StoppedState
		service.PID = -1
	}
	return s, service
}

func TestServiceState(t *testing.T) {
	active, activeService := getServiceForTest(t, true)
	inactive, inactiveService := getServiceForTest(t, false)

	tests := []struct {
		name    string
		service string
		want    *Service
		wantErr bool
	}{
		{"not_found", "not_found", &Service{ProcName: "", PID: -1, State: NotFoundState}, true},
		{"active", active, activeService, false},
		{"inactive", inactive, inactiveService, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ServiceState(tt.service)
			if (err != nil) != tt.wantErr {
				t.Errorf("ServiceState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.ProcName != tt.want.ProcName || got.State != tt.want.State {
				t.Errorf("ServiceState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServiceChecker_Status(t *testing.T) {
	failCount := 2
	checkCount := 3
	resetCount := 2

	active, _ := getServiceForTest(t, true)
	inactive, _ := getServiceForTest(t, false)

	tests := []struct {
		name        string
		service     string
		want        checker.State
		wantMetrics []string
	}{
		{
			name: "not_found", service: "not_found", want: checker.ErrorState,
			wantMetrics: []string{"systemd.not_found"},
		},
		{
			name: "active", service: active, want: checker.SuccessState,
			wantMetrics: []string{"systemd." + active},
		},
		{
			name: "inactive", service: inactive, want: checker.ErrorState,
			wantMetrics: []string{"systemd." + inactive},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServiceChecker(tt.service, failCount, checkCount, resetCount)
			for i := 0; i < checkCount+1; i++ {
				got, _ := s.Status(0)
				want := checker.CollectingState
				if i >= checkCount-1 {
					want = tt.want
				}
				if got != want {
					t.Errorf("Step %d ServiceChecker.Status() got = %v, want %v", i, got, want)
				}
				metrics := s.Metrics()
				for j := range metrics {
					if metrics[j].Name != tt.wantMetrics[j] || metrics[j].Value != strconv.Itoa(int(want)) {
						t.Errorf("Step %d ServiceChecker.Metrics()[%d] got = %v, want %v", i, j, metrics[j], tt.wantMetrics[j])
					}
				}
			}
		})
	}
}
