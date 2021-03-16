package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/msaf1980/relaymon/pkg/checker"
	"gopkg.in/yaml.v2"
)

type CarbonCRelay struct {
	Config   string   `yaml:"config"`
	Required []string `yaml:"required"`
}

// Config structure
type Config struct {
	LogLevel      string        `yaml:"log_level"`
	CheckInterval time.Duration `yaml:"check_interval"`

	CheckCount int `yaml:"check_count"`
	FailCount  int `yaml:"fail_count"`
	ResetCount int `yaml:"reset_count"`

	NetTimeout time.Duration `yaml:"net_timeout"`

	ErrorCmd   string `yaml:"error_cmd"`
	SuccessCmd string `yaml:"success_cmd"`

	Iface string   `yaml:"iface"`
	IPs   []string `yaml:"ips"`

	CarbonCRelay CarbonCRelay `yaml:"carbon_c_relay"`

	Services []string `yaml:"services"`

	Service string `yaml:"service"`

	Relay    string `yaml:"graphite_relay"`
	Prefix   string `yaml:"prefix"`
	Hostname string `yaml:"hostname"`
}

func defaultConfig() *Config {
	cfg := &Config{
		LogLevel:      "INFO",
		CheckInterval: 10 * time.Second,
		CheckCount:    6,
		FailCount:     3,
		ResetCount:    4,
		NetTimeout:    1 * time.Second,
		Iface:         "lo",
		IPs:           []string{},
		Services:      []string{},
		CarbonCRelay:  CarbonCRelay{Required: []string{}},
		Relay:         "127.0.0.1",
		Prefix:        "graphite.relaymon",
		Hostname:      "",
		Service:       "relaymon",
	}

	return cfg
}

// LoadConfig load config fle
func LoadConfig(configFile string, overrideLogLevel string) (*Config, error) {
	cfg := defaultConfig()

	yml, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yml, cfg)
	if err != nil {
		return nil, err
	}

	if len(overrideLogLevel) > 0 {
		cfg.LogLevel = overrideLogLevel
	}

	if len(cfg.Iface) == 0 {
		return nil, fmt.Errorf("configuration: iface empthy")
	}
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("configuration: services empthy")
	}
	if len(cfg.ErrorCmd) == 0 && len(cfg.IPs) == 0 {
		return nil, fmt.Errorf("configuration: error_cmd or ips empthy")
	}
	if len(cfg.SuccessCmd) == 0 && len(cfg.IPs) == 0 {
		return nil, fmt.Errorf("configuration: recovery_cmd or ips empthy")
	}
	if len(cfg.Hostname) == 0 {
		var err error
		cfg.Hostname, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("configuration: can't get hostname, %s", err.Error())
		}
	}
	cfg.Prefix += "." + checker.Strip(cfg.Hostname)

	return cfg, nil
}
