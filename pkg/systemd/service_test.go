package systemd

import (
	"testing"
)

func TestServiceState(t *testing.T) {
	tests := []struct {
		name    string
		want    *Service
		wantErr bool
	}{
		{"not_found", &Service{ProcName: "", PID: -1, State: NotFoundState}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ServiceState(tt.name)
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
