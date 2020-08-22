package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	carboncrelay "github.com/msaf1980/relaymon/pkg/carbon_c_relay"
	"github.com/msaf1980/relaymon/pkg/carbonnetwork"
	"github.com/msaf1980/relaymon/pkg/checker"
	"github.com/msaf1980/relaymon/pkg/systemd"

	config "github.com/msaf1980/relaymon/config/relaymon"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var running bool = true

type CheckStatus struct {
	Checker checker.Checker
	Status  checker.State
}

func logStatus(s checker.State, c *CheckStatus, errs []error) {
	if s != c.Status {
		switch s {
		case checker.CollectingState:
			log.Info().Str("service", c.Checker.Name()).Msg("collecting state")
		case checker.SuccessState:
			log.Info().Str("service", c.Checker.Name()).Msg("state changed to success")
		case checker.WarnState:
			if len(errs) > 0 {
				for i := range errs {
					log.Error().Str("service", c.Checker.Name()).Msg(errs[i].Error())
				}
			}
			log.Warn().Str("service", c.Checker.Name()).Msg("state changed to warning")
		case checker.ErrorState:
			if len(errs) > 0 {
				for i := range errs {
					log.Error().Str("service", c.Checker.Name()).Msg(errs[i].Error())
				}
			}
			log.Error().Str("service", c.Checker.Name()).Msg("state changed to error")
		case checker.NotFoundState:
			log.Error().Str("service", c.Checker.Name()).Msg("not found")
		default:
			if len(errs) > 0 {
				for i := range errs {
					log.Error().Str("service", c.Checker.Name()).Msg(errs[i].Error())
				}
			}
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
	//version := flag.String("version", "", "version")
	flag.Parse()

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
	log := zerolog.New(multi).With().Timestamp().Logger()

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

	checkers := make([]CheckStatus, len(cfg.Services))
	for i := range checkers {
		checkers[i].Checker = systemd.NewServiceChecker(cfg.Services[i], cfg.FailCount, cfg.CheckCount, cfg.ResetCount)
	}

	netCheckers := make([]CheckStatus, 0)

	// carbon-c-relay

	if cfg.CarbonCRelay.Config != "" {
		clusters, err := carboncrelay.Clusters(cfg.CarbonCRelay.Config, cfg.CarbonCRelay.Required)
		if err != nil {
			log.Error().Str("carbon-c-relay", "load config").Msg(err.Error())
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

		// services
		success := 0
		for i := range checkers {
			s, errs := checkers[i].Checker.Status()
			if s == checker.ErrorState {
				stepStatus = checker.ErrorState
			} else if s == checker.SuccessState {
				success++
			}

			logStatus(s, &checkers[i], errs)
		}

		success = 0
		for i := range netCheckers {
			s, errs := netCheckers[i].Checker.Status()
			if s == checker.ErrorState {
				stepStatus = checker.ErrorState
			} else if s == checker.SuccessState {
				success++
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
				if len(cfg.ErrorCmd) > 0 {
					out, err := execute(cfg.ErrorCmd)
					if err == nil {
						log.Error().Str("action", "down").Msg(out)
					} else {
						log.Error().Str("action", "down").Str("error", err.Error()).Msg(out)
					}

				}
			} else if stepStatus == checker.SuccessState {
				// checks success
				log.Info().Str("action", "up").Msg("go to success state")
				status = checker.SuccessState
				if len(cfg.SuccessCmd) > 0 {
					out, err := execute(cfg.ErrorCmd)
					if err == nil {
						log.Info().Str("action", "up").Msg(out)
					} else {
						log.Error().Str("action", "up").Str("error", err.Error()).Msg(out)
					}
				}
			}
		}

		time.Sleep(cfg.CheckInterval)
	}
}
