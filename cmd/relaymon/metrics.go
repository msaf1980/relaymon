package main

import (
	"time"

	lockfree_queue "github.com/msaf1980/go-lockfree-queue"
	graphite "github.com/msaf1980/graphite-golang"
)

type GraphiteQueue struct {
	queue     *lockfree_queue.Queue
	graphite  *graphite.Graphite
	batchSend int

	failed  bool
	running bool
}

// GraphiteInit init metric sender
func GraphiteInit(address string, prefix string, queueSize int, batchSend int) (*GraphiteQueue, error) {
	return new(GraphiteQueue).init(address, prefix, queueSize, batchSend)
}

func (g *GraphiteQueue) init(address string, prefix string, queueSize int, batchSend int) (*GraphiteQueue, error) {
	if len(address) > 0 {
		var err error
		g.queue = lockfree_queue.NewQueue(queueSize)
		if batchSend < 1 {
			g.batchSend = 1
		} else {
			g.batchSend = batchSend
		}
		g.graphite, err = graphite.NewGraphiteWithMetricPrefix(address, prefix)
		return g, err
	} else {
		return g, nil
	}
}

// Put metric to queue
func (g *GraphiteQueue) Put(name, value string, timestamp int64) {
	if g.queue == nil {
		return
	}
	m := graphite.NewMetricPtr(name, value, timestamp)
	if !g.queue.Put(m) {
		// drop last two elements and try put again
		g.queue.Get()
		g.queue.Get()
		g.queue.Put(m)
	}
}

// Run goroutune for queue read and send metrics
func (g *GraphiteQueue) Run() {
	if g.queue == nil {
		return
	}
	g.running = true
	go func() {
		metrics := make([]*graphite.Metric, g.batchSend)
		i := 0
		nextSend := false
		for g.running {
			if i == g.batchSend || nextSend {
				if !g.graphite.IsConnected() {
					err := g.graphite.Connect()
					if err != nil {
						_ = g.graphite.Disconnect()
						if !g.failed {
							g.failed = true
							log.Error().Str("relaymon", "metric").Msg(err.Error())
						}
						time.Sleep(1 * time.Second)
						continue
					}
				}
				err := g.graphite.SendMetricPtrs(metrics[0:i])
				if err == nil {
					i = 0
					nextSend = false
					if g.failed {
						g.failed = false
						log.Info().Str("relaymon", "metric").Msg("metrics sended")
					}
				} else {
					if !g.failed {
						g.failed = true
						log.Error().Str("relaymon", "metric").Msg(err.Error())
					}
					continue
				}
			}
			if i < g.batchSend {
				m, _ := g.queue.Get()
				if m != nil {
					metrics[i] = m.(*graphite.Metric)
					i++
				} else {
					m, _ := g.queue.Get()
					if m != nil {
						metrics[i] = m.(*graphite.Metric)
						i++
					} else if i == 0 {
						time.Sleep(1 * time.Second)
					} else {
						nextSend = true
					}
				}
			}
		}
	}()
}

// Stop goroutune for queue read and send metrics
func (g *GraphiteQueue) Stop() {
	g.running = false
}
