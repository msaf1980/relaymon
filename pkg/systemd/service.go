package systemd

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/msaf1980/relaymon/pkg/checker"
	"github.com/msaf1980/relaymon/pkg/linuxproc"
)

const (
	systemdPID = 1
)

var (
	rePID = regexp.MustCompile(` Main PID: +([0-9]+) (\([a-zA-Z0-9_\-]+\)?)`)
)

// State systemd service state
type State int8

const (
	// StartedState started
	StartedState State = iota

	// StoppedState stopped
	StoppedState

	// FailedState failed
	FailedState

	// NotFoundState not found
	NotFoundState

	// UnknownState can't get state
	UnknownState
)

// Service systemd service status
type Service struct {
	ProcName  string
	PID       int64
	State     State
	StartTime time.Time
}

// $ sudo systemctl status sshd
// ● sshd.service - OpenSSH server daemon
//    Loaded: loaded (/usr/lib/systemd/system/sshd.service; enabled; vendor preset: enabled)
//    Active: active (running) since Thu 2020-08-20 23:11:59 +05; 52s ago
//      Docs: man:sshd(8)
//            man:sshd_config(5)
//  Main PID: 14373 (sshd)
//     Tasks: 1 (limit: 4915)
//    Memory: 1.0M
//    CGroup: /system.slice/sshd.service
//            └─14373 /usr/sbin/sshd -D -oCiphers=aes256-gcm@openssh.com,chacha20-poly1305@openssh.com,aes256-ctr,aes256-cbc,aes128-gcm@openssh.com,aes128-ctr,aes128-cbc -oMACs=hmac-sha2-256-etm@openssh.com,hmac-sha1-etm@openssh.com,umac-1>

// Aug 20 23:11:59 test.test.int systemd[1]: Starting OpenSSH server daemon...
// Aug 20 23:11:59 test.test.int sshd[14373]: Server listening on 0.0.0.0 port 22.
// Aug 20 23:11:59 test.test.int sshd[14373]: Server listening on :: port 22.
// Aug 20 23:11:59 test.test.int systemd[1]: Started OpenSSH server daemon.

// ServiceState return SystemdService
func ServiceState(name string) (*Service, error) {
	service := &Service{PID: -1}
	cmd := exec.Command("/bin/systemctl", "status", name)
	var stdOut bytes.Buffer
	cmd.Stdout = &stdOut
	err := cmd.Run()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			switch exitErr.ExitCode() {
			case 3:
				service.State = StoppedState
				return service, fmt.Errorf("service %s stopped", name)
			case 4:
				service.State = NotFoundState
				return service, fmt.Errorf("service %s not found", name)
			default:
				service.State = FailedState
				return service, fmt.Errorf("service %s failed", name)
			}
		} else {
			service.State = UnknownState
			return service, fmt.Errorf("service %s %s", name, err.Error())
		}
	}
	matches := rePID.FindStringSubmatch(stdOut.String())
	if len(matches) == 0 {
		err = fmt.Errorf("service %s can't extract pid", name)
	} else {
		service.PID, err = strconv.ParseInt(matches[1], 10, 64)
		service.ProcName = matches[2]
	}

	return service, err
}

// ServiceChecker systemd service (implement pkg/checker/Checker interface)
type ServiceChecker struct {
	name string

	Process *linuxproc.Proc

	// check results
	failed  int
	success int
	checked int

	// check thresholds
	failCount  int
	checkCount int
	resetCount int
}

// NewServiceChecker return new systemd service instance
func NewServiceChecker(name string, failCount int, checkCount int, resetCount int) *ServiceChecker {
	service := &ServiceChecker{
		name:       name,
		failCount:  failCount,
		checkCount: checkCount,
		resetCount: resetCount,
	}
	return service
}

// Name get service name
func (s *ServiceChecker) Name() string {
	return s.name
}

func (s *ServiceChecker) procExit() (bool, error) {
	proc, err := linuxproc.ProcInfo(s.Process.PID)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		} else {
			return false, err
		}
	}

	return *proc != *s.Process, nil
}

// Status get result of service status check
func (s *ServiceChecker) Status() (checker.State, []error) {
	needRecheck := false
	successCheck := false

	if s.Process == nil {
		needRecheck = true
	} else {
		exit, procErr := s.procExit()
		if procErr != nil {
			s.failed = 0
			s.success = 0
			s.checked = 0
			return checker.UnknownState, []error{procErr}
		} else if exit {
			// proc with this pid changed
			if s.failed < math.MaxInt32 {
				s.failed++
			}
			s.Process = nil
		} else {
			successCheck = true
		}
	}

	if needRecheck {
		service, err := ServiceState(s.name)
		if err != nil {
			switch service.State {
			case UnknownState:
				s.failed = 0
				s.success = 0
				s.checked = 0
				return checker.UnknownState, []error{err}
			default:
				if s.failed < math.MaxInt32 {
					s.failed++
				}
			}
		} else {
			proc, procErr := linuxproc.ProcInfo(service.PID)
			if procErr != nil {
				if os.IsNotExist(procErr) {
					if s.failed < math.MaxInt32 {
						s.failed++
					}
				} else {
					s.failed = 0
					s.success = 0
					s.checked = 0
					return checker.UnknownState, []error{procErr}
				}
			} else {
				if proc.PPID != systemdPID || proc.ProcName != service.ProcName {
					if s.failed < math.MaxInt32 {
						s.failed++
					}
				} else {
					successCheck = true
					s.Process = proc
				}
			}
		}

	}

	if s.checked < math.MaxInt32 {
		s.checked++
	}

	if successCheck {
		if s.success < math.MaxInt32 {
			s.success++
		}
		if s.failed > 0 && s.success >= s.resetCount {
			s.failed = 0
		}
	} else if s.success > 0 {
		s.success = 0
	}
	if s.checked < s.checkCount {
		return checker.CollectingState, nil
	} else if s.failed > 0 {
		if s.failed >= s.failCount {
			return checker.ErrorState, nil
		}
		return checker.WarnState, nil
	}
	return checker.SuccessState, nil
}
