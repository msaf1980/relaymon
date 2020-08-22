package linuxproc

import (
	"os"
	"reflect"
	"testing"

	"github.com/msaf1980/relaymon/pkg/linuxstat"
)

func TestProcInfo(t *testing.T) {
	type args struct {
		pid int64
	}

	proc1 := &Proc{PID: 1, PPID: 0, ProcName: "(systemd)"}
	_, _, proc1.StartTime, _ = linuxstat.FileStatTimes("/proc/1")

	tests := []struct {
		name    string
		args    args
		want    *Proc
		wantErr bool
	}{
		{"init process", args{1}, proc1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcInfo(tt.args.pid)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProcInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcInfoNotExist(t *testing.T) {
	tests := []struct {
		name string
		pid  int64
	}{
		{"not found", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProcInfo(tt.pid)
			if err == nil {
				t.Errorf("ProcInfo() process not exist")
			} else if !os.IsNotExist(err) {
				t.Errorf("ProcInfo() error = %v", err)
			}
		})
	}
}
