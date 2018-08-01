// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/mgo.v2"
)

// MgoStatsCollector is a prometheus.Collector that reports the values
// provided by mgo.GetStats.
type MgoStatsCollector struct {
	clusters    prometheus.Gauge
	masterConns prometheus.Gauge
	slaveConns  prometheus.Gauge
	// The following three metrics are actually incremental, but
	// we're using Gauges here because the prometheus.Counter interface
	// implies knowledge of the diff in reported values, which we don't have.
	sentOps      prometheus.Gauge
	receivedOps  prometheus.Gauge
	receivedDocs prometheus.Gauge

	socketsAlive prometheus.Gauge
	socketsInUse prometheus.Gauge
	socketRefs   prometheus.Gauge
}

// Check implementation of prometheus.Collector interface.
var _ prometheus.Collector = (*MgoStatsCollector)(nil)

// NewMgoStatsCollector creates a MgoStatsCollector for the given
// namespace (which may be empty).
func NewMgoStatsCollector(namespace string) *MgoStatsCollector {
	// Enable stats in the mgo driver.
	mgo.SetStats(true)
	return &MgoStatsCollector{
		clusters: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "clusters",
			Help:      "Number of alive clusters.",
		}),
		masterConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "master_connections",
			Help:      "Number of master connections.",
		}),
		slaveConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "slave_connections",
			Help:      "Number of slave connections.",
		}),
		sentOps: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "sent_operations",
			Help:      "Number of operations sent.",
		}),
		receivedOps: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "received_operations",
			Help:      "Number of operations received.",
		}),
		receivedDocs: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "received_documents",
			Help:      "Number of documents received.",
		}),
		socketsAlive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "sockets_alive",
			Help:      "Number of alive sockets.",
		}),
		socketsInUse: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "sockets_in_use",
			Help:      "Number of in use sockets.",
		}),
		socketRefs: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mgo",
			Name:      "socket_references",
			Help:      "Number of references to sockets.",
		}),
	}
}

// Describe implements prometheus.Collector.Describe.
func (m *MgoStatsCollector) Describe(c chan<- *prometheus.Desc) {
	c <- m.clusters.Desc()
	c <- m.masterConns.Desc()
	c <- m.slaveConns.Desc()
	c <- m.sentOps.Desc()
	c <- m.receivedOps.Desc()
	c <- m.receivedDocs.Desc()
	c <- m.socketsAlive.Desc()
	c <- m.socketsInUse.Desc()
	c <- m.socketRefs.Desc()
}

// Collect implements prometheus.Collector.Collect.
func (m *MgoStatsCollector) Collect(c chan<- prometheus.Metric) {
	stats := mgo.GetStats()
	m.clusters.Set(float64(stats.Clusters))
	c <- m.clusters
	m.masterConns.Set(float64(stats.MasterConns))
	c <- m.masterConns
	m.slaveConns.Set(float64(stats.SlaveConns))
	c <- m.slaveConns
	m.sentOps.Set(float64(stats.SentOps))
	c <- m.sentOps
	m.receivedOps.Set(float64(stats.ReceivedOps))
	c <- m.receivedOps
	m.receivedDocs.Set(float64(stats.ReceivedDocs))
	c <- m.receivedDocs
	m.socketsAlive.Set(float64(stats.SocketsAlive))
	c <- m.socketsAlive
	m.socketsInUse.Set(float64(stats.SocketsInUse))
	c <- m.socketsInUse
	m.socketRefs.Set(float64(stats.SocketRefs))
	c <- m.socketRefs
}
