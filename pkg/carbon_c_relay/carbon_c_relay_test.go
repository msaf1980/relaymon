package carboncrelay

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/msaf1980/relaymon/pkg/carbonnetwork"
)

func TestClusters(t *testing.T) {
	tests := []struct {
		config   string
		required []string
		want     []carbonnetwork.Cluster
	}{
		{
			"carbon-c-relay.conf",
			[]string{"test2"},
			[]carbonnetwork.Cluster{
				{Name: "test1", Endpoints: []string{"test1:2003", "test2:2005"}, Required: false},
				{Name: "test2", Endpoints: []string{"test3:2003", "test4:2005"}, Required: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.config, func(t *testing.T) {
			got, err := Clusters(tt.config, tt.required)
			if err != nil {
				t.Errorf("Clusters() error = %v", err)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("Clusters() got size = %d, want %d", len(got), len(tt.want))
			} else {
				for i := range got {
					if !reflect.DeepEqual(got[i].Endpoints, tt.want[i].Endpoints) {
						t.Errorf("Clusters()[%d].Endpoints got = %v, want %v", i, got[i].Endpoints, tt.want[i].Endpoints)
					}
					if got[i].Required != tt.want[i].Required {
						t.Errorf("Clusters()[%d].Required got = %s, want %s", i, strconv.FormatBool(got[i].Required), strconv.FormatBool(tt.want[i].Required))
					}
				}

			}
		})
	}
}
