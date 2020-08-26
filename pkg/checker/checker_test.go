package checker

import (
	"fmt"
	"testing"
)

func TestErrorChanged(t *testing.T) {
	tests := []struct {
		name     string
		previous error
		current  error
		result   bool
	}{
		{"nil == nil", nil, nil, false},
		{"nil != 'Test'", nil, fmt.Errorf("test"), true},
		{"'test' != nil", fmt.Errorf("test"), nil, true},
		{"'test' == 'test", fmt.Errorf("test"), fmt.Errorf("test"), false},
		{"'test1' == 'test2", fmt.Errorf("test1"), fmt.Errorf("test2"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorChanged(tt.previous, tt.current); got != tt.result {
				t.Errorf("ErrorChanged() = %v, want %v", got, tt.result)
			}
		})
	}
}
