package linuxproc

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/msaf1980/relaymon/pkg/linuxstat"
)

// Proc process info
type Proc struct {
	PID       int64
	PPID      int64
	ProcName  string
	StartTime time.Time
}

// ProcInfo return process info
func ProcInfo(pid int64) (*Proc, error) {
	proc := &Proc{}
	statProc := fmt.Sprintf("/proc/%d", pid)
	file, err := os.Open(statProc + "/stat")
	if err != nil {
		return nil, err
	}
	proc.PID = pid

	_, _, proc.StartTime, err = linuxstat.FileStatTimes(statProc)
	if err != nil {
		return proc, err
	}

	b, err := ioutil.ReadAll(file)
	if err != nil {
		return proc, err
	}
	fields := strings.Split(string(b), " ")
	if len(fields) < 22 {
		return proc, fmt.Errorf("can't get pid stat")
	}
	if len(fields[1]) > 3 {
		start := 0
		if fields[1][0] == '(' {
			start = 1
		}
		end := len(fields[1])
		if fields[1][end-1] == ')' {
			end--
		}
		proc.ProcName = fields[1][start:end]
	} else {
		proc.ProcName = fields[1]
	}
	proc.PPID, err = strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return proc, err
	}

	return proc, err
}
