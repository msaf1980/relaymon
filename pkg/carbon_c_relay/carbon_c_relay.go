package carboncrelay

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/msaf1980/relaymon/pkg/carbonnetwork"
)

var (
	skipList0 = map[string]bool{"forward": true, "any_of": true, "failover": true, "useall": true,
		"carbon_ch": true, "fnv1a_ch": true, "jump_fnv1a_ch": true, "lb": true,
		"dynamic": true}
	skipList1 = map[string]bool{"replication": true}
	stopList  = map[string]bool{"proto": true, "type": true, "transport": true}
)

func clusterEndpoints(fields []string, required map[string]bool, testPrefix string, timeout time.Duration) (*carbonnetwork.Cluster, error) {
	if len(fields) < 4 {
		return nil, fmt.Errorf("incomplete cluster")
	}
	var err error

	name := fields[1]
	_, ok := required[name]
	cluster := carbonnetwork.NewCluster(name, ok, testPrefix, timeout)
	i := 2
	for i < len(fields) {
		if fields[i] == "file" {
			return nil, nil
		}
		_, ok := skipList0[fields[i]]
		if ok {
			i++
			continue
		}

		_, ok = skipList1[fields[i]]
		if ok {
			i += 2
			continue
		}

		_, ok = stopList[fields[i]]
		if ok {
			break
		}

		endpoint := strings.Split(fields[i], "=")
		endpoint = strings.Split(endpoint[0], ":")
		if len(endpoint) == 1 {
			cluster.Append(endpoint[0] + ":2003")
		} else {
			cluster.Append(endpoint[0] + ":" + endpoint[1])
		}

		i++
	}

	if len(cluster.Endpoints) == 0 {
		err = fmt.Errorf("empthy cluster %s", cluster.Name)
	}

	return cluster, err
}

// Clusters parse config and return clusters
func Clusters(config string, required []string, testPrefix string, timeout time.Duration) ([]*carbonnetwork.Cluster, error) {
	clusters := make([]*carbonnetwork.Cluster, 0, 2)
	file, err := os.Open(config)
	if err != nil {
		return clusters, err
	}

	defer file.Close()

	r := map[string]bool{}
	for i := range required {
		r[required[i]] = true
	}

	reader := bufio.NewReader(file)

	found := false
	clusterFields := make([]string, 0)
	for {
		line, err := reader.ReadString('\n')
		line = strings.Split(line, "#")[0]
		line = strings.TrimRight(line, "\n")
		if found {
			fields := strings.Split(line, " ")
			for i := range fields {
				if fields[i] == "" {
					continue
				} else if fields[i] == ";" {
					found = false
					break
				}
				clusterFields = append(clusterFields, fields[i])
			}
		} else if strings.HasPrefix(line, "cluster ") {
			if len(clusterFields) > 0 {
				cluster, err := clusterEndpoints(clusterFields, r, testPrefix, timeout)
				if cluster != nil && err == nil {
					clusters = append(clusters, cluster)
				}
				clusterFields = make([]string, 0)
			}
			fields := strings.Split(line, " ")
			for i := range fields {
				if fields[i] == "" {
					continue
				} else if fields[i] == ";" {
					break
				}
				clusterFields = append(clusterFields, fields[i])
			}
			found = true
		}
		if err != nil {
			break
		}
	}

	if len(clusterFields) > 0 {
		cluster, err := clusterEndpoints(clusterFields, r, testPrefix, timeout)
		if cluster != nil && err == nil {
			clusters = append(clusters, cluster)
		}
	}

	return clusters, nil
}
