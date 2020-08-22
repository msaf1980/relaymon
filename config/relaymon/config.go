package config

import (
	"fmt"
	"io/ioutil"
	"time"

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

	RecoveryInterval time.Duration `yaml:"recovery_interval"`

	NetTimeout time.Duration `yaml:"net_timeout"`

	ErrorCmd    string `yaml:"error_cmd"`
	RecoveryCmd string `yaml:"recovery_cmd"`

	Iface string   `yaml:"iface"`
	IPs   []string `yaml:"ips"`

	CarbonCRelay CarbonCRelay `yaml:"carbon_c_relay"`

	Services []string `yaml:"services"`

	Relay  string `yaml:"graphite_relay"`
	Prefix string `yaml:"prefix"`
}

func defaultConfig() *Config {
	cfg := &Config{
		LogLevel:         "INFO",
		CheckInterval:    10 * time.Second,
		CheckCount:       6,
		FailCount:        3,
		ResetCount:       4,
		RecoveryInterval: 120 * time.Second,
		NetTimeout:       1 * time.Second,
		Iface:            "lo",
		IPs:              []string{},
		Services:         []string{},
		CarbonCRelay:     CarbonCRelay{Required: []string{}},
		Relay:            "127.0.0.1",
		Prefix:           "graphite.relaymon",
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
	if len(cfg.RecoveryCmd) == 0 && len(cfg.IPs) == 0 {
		return nil, fmt.Errorf("configuration: recovery_cmd or ips empthy")
	}

	return cfg, nil
}
