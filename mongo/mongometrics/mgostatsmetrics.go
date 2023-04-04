// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongometrics

import (
	"sync"

	"github.com/juju/mgo/v3"
	"github.com/prometheus/client_golang/prometheus"
)

// MgoStatsCollector is a prometheus.Collector that collects metrics based
// on mgo stats.
type MgoStatsCollector struct {
	getStats func() (current, previous stats)

	clustersGauge       prometheus.Gauge
	masterConnsGauge    prometheus.Gauge
	slaveConnsGauge     prometheus.Gauge
	sentOpsCounter      prometheus.Counter
	receivedOpsCounter  prometheus.Counter
	receivedDocsCounter prometheus.Counter
	socketsAliveGauge   prometheus.Gauge
	socketsInuseGauge   prometheus.Gauge
	socketRefsGauge     prometheus.Gauge
}

type stats struct {
	Clusters     int
	MasterConns  int
	SlaveConns   int
	SentOps      int
	ReceivedOps  int
	ReceivedDocs int
	SocketsAlive int
	SocketsInUse int
	SocketRefs   int
}

// NewMgoStatsCollector returns a new MgoStatsCollector.
func NewMgoStatsCollector(getCurrentStats func() mgo.Stats) *MgoStatsCollector {
	// We need to track previous statistics so we can
	// compute the delta for counter metrics.
	var mu sync.Mutex
	var prevStats stats
	getStats := func() (current, previous stats) {
		mu.Lock()
		defer mu.Unlock()
		previous = prevStats
		currentStats := getCurrentStats()
		current = stats{
			Clusters:     currentStats.Clusters,
			MasterConns:  currentStats.MasterConns,
			SlaveConns:   currentStats.SlaveConns,
			SentOps:      currentStats.SentOps,
			ReceivedOps:  currentStats.ReceivedOps,
			ReceivedDocs: currentStats.ReceivedDocs,
			SocketsAlive: currentStats.SocketsAlive,
			SocketsInUse: currentStats.SocketsInUse,
			SocketRefs:   currentStats.SocketRefs,
		}
		prevStats = current
		return current, previous
	}

	return &MgoStatsCollector{
		getStats: getStats,

		clustersGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "clusters",
			Help:      "Current number of clusters",
		}),
		masterConnsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "master_conns",
			Help:      "Current number of master conns",
		}),
		slaveConnsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "slave_conns",
			Help:      "Current number of slave conns",
		}),
		sentOpsCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "mgo",
			Name:      "sent_ops_total",
			Help:      "Total number of sent ops",
		}),
		receivedOpsCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "mgo",
			Name:      "received_ops_total",
			Help:      "Total number of received ops",
		}),
		receivedDocsCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "mgo",
			Name:      "received_docs_total",
			Help:      "Total number of received docs",
		}),
		socketsAliveGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "sockets_alive",
			Help:      "Current number of sockets alive",
		}),
		socketsInuseGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "sockets_inuse",
			Help:      "Current number of sockets in use",
		}),
		socketRefsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "socket_refs",
			Help:      "Current number of sockets referenced",
		}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *MgoStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.clustersGauge.Describe(ch)
	c.masterConnsGauge.Describe(ch)
	c.slaveConnsGauge.Describe(ch)
	c.sentOpsCounter.Describe(ch)
	c.receivedOpsCounter.Describe(ch)
	c.receivedDocsCounter.Describe(ch)
	c.socketsAliveGauge.Describe(ch)
	c.socketsInuseGauge.Describe(ch)
	c.socketRefsGauge.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *MgoStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats, prevStats := c.getStats()

	c.clustersGauge.Set(float64(stats.Clusters))
	c.masterConnsGauge.Set(float64(stats.MasterConns))
	c.slaveConnsGauge.Set(float64(stats.SlaveConns))
	if n := stats.SentOps - prevStats.SentOps; n > 0 {
		c.sentOpsCounter.Add(float64(n))
	}
	if n := stats.ReceivedOps - prevStats.ReceivedOps; n > 0 {
		c.receivedOpsCounter.Add(float64(n))
	}
	if n := stats.ReceivedDocs - prevStats.ReceivedDocs; n > 0 {
		c.receivedDocsCounter.Add(float64(n))
	}
	c.socketsAliveGauge.Set(float64(stats.SocketsAlive))
	c.socketsInuseGauge.Set(float64(stats.SocketsInUse))
	c.socketRefsGauge.Set(float64(stats.SocketRefs))

	c.clustersGauge.Collect(ch)
	c.masterConnsGauge.Collect(ch)
	c.slaveConnsGauge.Collect(ch)
	c.sentOpsCounter.Collect(ch)
	c.receivedOpsCounter.Collect(ch)
	c.receivedDocsCounter.Collect(ch)
	c.socketsAliveGauge.Collect(ch)
	c.socketsInuseGauge.Collect(ch)
	c.socketRefsGauge.Collect(ch)
}
