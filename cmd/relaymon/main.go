package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	carboncrelay "github.com/msaf1980/relaymon/pkg/carbon_c_relay"
	"github.com/msaf1980/relaymon/pkg/carbonnetwork"
	"github.com/msaf1980/relaymon/pkg/checker"
	"github.com/msaf1980/relaymon/pkg/netconf"
	"github.com/msaf1980/relaymon/pkg/systemd"

	config "github.com/msaf1980/relaymon/config/relaymon"

	"github.com/rs/zerolog"
)

var (
	running bool = true
	log     zerolog.Logger
	version string
)

type CheckStatus struct {
	Checker checker.Checker
	Status  checker.State
}

func logStatus(s checker.State, c *CheckStatus, events []string) {
	if len(events) > 0 {
		for i := range events {
			log.Info().Str("service", c.Checker.Name()).Msg(events[i])
		}
	}
	if s != c.Status {
		switch s {
		case checker.CollectingState:
			log.Info().Str("service", c.Checker.Name()).Msg("collecting state")
		case checker.SuccessState:
			log.Info().Str("service", c.Checker.Name()).Msg("state changed to success")
		case checker.WarnState:
			log.Warn().Str("service", c.Checker.Name()).Msg("state changed to warning")
		case checker.ErrorState:
			log.Error().Str("service", c.Checker.Name()).Msg("state changed to error")
		case checker.NotFoundState:
			log.Error().Str("service", c.Checker.Name()).Msg("not found")
		default:
			log.Error().Str("service", c.Checker.Name()).Msg("unknown state")
		}
		c.Status = s
	}
}

func execute(command string) (string, error) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("command timeout")
	} else if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			err = fmt.Errorf("command exit with %d", exitErr.ExitCode())
		} else {
			err = fmt.Errorf("command execute error with %s", err.Error())
		}
	}
	return string(out), err
}

func main() {
	configFile := flag.String("config", "/etc/relaymon.yml", "config file (in YAML)")
	logLevel := flag.String("loglevel", "", "override loglevel")
	evict := flag.Bool("evict", false, "stop relaymon, remove ips and run error command (without run daemon)")
	ver := flag.Bool("version", false, "version")
	flag.Parse()

	if *ver {
		fmt.Printf("relaymon version %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.LoadConfig(*configFile, *logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration load: %s\n", err.Error())
		os.Exit(1)
	}

	level, err := zerolog.ParseLevel(strings.ToLower(cfg.LogLevel))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log_level: %s\n", cfg.LogLevel)
		os.Exit(1)
	}
	zerolog.SetGlobalLevel(level)
	multi := zerolog.MultiLevelWriter(os.Stdout)
	log = zerolog.New(multi).With().Timestamp().Logger()

	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-signalChannel
		switch sig {
		case os.Interrupt:
			running = false
		case syscall.SIGTERM:
			running = false
		}
	}()

	addrs := make([]*net.IPNet, len(cfg.IPs))
	for i := range cfg.IPs {
		var ip net.IP
		ip, addrs[i], err = net.ParseCIDR(cfg.IPs[i])
		if err != nil {
			log.Fatal().Msg(err.Error())
		}
		addrs[i].IP = ip
	}

	if *evict {
		rc := 0

		out, err := execute("systemctl stop " + cfg.Service)
		if err == nil {
			log.Info().Str("action", "stop").Str("type", "cmd").Msg(out)
		} else {
			log.Error().Str("action", "stop").Str("type", "cmd").Str("error", err.Error()).Msg(out)
			rc++
		}

		if len(addrs) > 0 {
			errs := netconf.IfaceAddrDel(cfg.Iface, addrs)
			if len(errs) > 0 {
				rc++
				for i := range errs {
					log.Error().Msg(errs[i].Error())
				}
			}
		}
		if len(cfg.ErrorCmd) > 0 {
			out, err := execute(cfg.ErrorCmd)
			if err == nil {
				log.Info().Str("action", "down").Str("type", "cmd").Msg(out)
			} else {
				log.Error().Str("action", "down").Str("type", "cmd").Str("error", err.Error()).Msg(out)
				rc++
			}
		}

		os.Exit(rc)
	}

	checkers := make([]CheckStatus, len(cfg.Services))
	for i := range checkers {
		checkers[i].Checker = systemd.NewServiceChecker(cfg.Services[i], cfg.FailCount, cfg.CheckCount, cfg.ResetCount)
	}

	netCheckers := make([]CheckStatus, 0)

	graphite, _ := GraphiteInit(cfg.Relay, cfg.Prefix, 4096, 14)
	graphite.Run()

	// carbon-c-relay
	if cfg.CarbonCRelay.Config != "" {
		clusters, err := carboncrelay.Clusters(cfg.CarbonCRelay.Config, cfg.CarbonCRelay.Required, cfg.Prefix, cfg.NetTimeout)
		if err != nil {
			log.Fatal().Str("carbon-c-relay", "load config").Msg(err.Error())
		} else {
			checker := carbonnetwork.NewNetworkChecker("carbon-c-relay clusters", clusters, cfg.NetTimeout, cfg.FailCount, cfg.CheckCount, cfg.ResetCount)
			if len(cfg.Relay) > 0 && len(cfg.Prefix) > 0 {
				checker.SetNotify(true)
			} else {
				checker.SetNotify(false)
			}
			netCheckers = append(netCheckers, CheckStatus{Checker: checker})
		}
	}

	status := checker.CollectingState
	checks := len(checkers) + len(netCheckers)
	for running {
		stepStatus := checker.CollectingState
		timestamp := time.Now().Unix()

		// services
		success := 0
		for i := range checkers {
			s, errs := checkers[i].Checker.Status(timestamp)
			if s == checker.ErrorState {
				stepStatus = checker.ErrorState
			} else if s == checker.SuccessState {
				success++
			}
			logStatus(s, &checkers[i], errs)

			metrics := checkers[i].Checker.Metrics()
			for k := range metrics {
				graphite.Put(metrics[k].Name, metrics[k].Value, timestamp)
			}
		}

		for i := range netCheckers {
			s, errs := netCheckers[i].Checker.Status(timestamp)
			if s == checker.ErrorState {
				stepStatus = checker.ErrorState
			} else if s == checker.SuccessState {
				success++
			}
			metrics := netCheckers[i].Checker.Metrics()
			for k := range metrics {
				graphite.Put(metrics[k].Name, metrics[k].Value, timestamp)
			}

			logStatus(s, &netCheckers[i], errs)
		}

		if success == checks {
			stepStatus = checker.SuccessState
		}

		if status != stepStatus {
			// status changed
			if stepStatus == checker.ErrorState {
				// checks failed
				log.Error().Str("action", "down").Msg("go to error state")
				status = checker.ErrorState
				if len(cfg.IPs) > 0 {
					errs := netconf.IfaceAddrDel(cfg.Iface, addrs)
					if len(errs) > 0 {
						for i := range errs {
							log.Error().Str("action", "up").Str("type", "network").Msg(errs[i].Error())
						}
					} else {
						log.Info().Str("action", "down").Str("type", "network").Msg("IP addresses deconfigured")
					}
				}
				if len(cfg.ErrorCmd) > 0 {
					out, err := execute(cfg.ErrorCmd)
					if err == nil {
						log.Info().Str("action", "down").Str("type", "cmd").Msg(out)
					} else {
						log.Error().Str("action", "down").Str("type", "cmd").Str("error", err.Error()).Msg(out)
					}

				}
			} else if stepStatus == checker.SuccessState {
				// checks success
				status = checker.SuccessState
				if len(cfg.IPs) > 0 {
					errs := netconf.IfaceAddrAdd(cfg.Iface, addrs)
					if len(errs) > 0 {
						status = checker.ErrorState
						for i := range errs {
							log.Error().Str("action", "up").Str("type", "network").Msg(errs[i].Error())
						}
					} else {
						log.Info().Str("action", "up").Str("type", "network").Msg("IP addresses configured")
					}
				}
				if len(cfg.SuccessCmd) > 0 {
					out, err := execute(cfg.SuccessCmd)
					if err == nil {
						log.Info().Str("action", "up").Str("type", "cmd").Msg(out)
					} else {
						status = checker.ErrorState
						log.Error().Str("action", "up").Str("type", "cmd").Str("error", err.Error()).Msg(out)
					}
				}
			}
		}

		graphite.Put("status", strconv.Itoa(int(stepStatus)), timestamp)
		if !running {
			break
		}
		time.Sleep(cfg.CheckInterval)
	}

	graphite.Stop()
	log.Info().Msg("shutdown")
}
