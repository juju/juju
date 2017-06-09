// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mgometrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/mgo.v2"
)

// Collector is a prometheus.Collector that collects metrics based
// on mgo stats.
type Collector struct {
	clustersGauge     prometheus.Gauge
	masterConnsGauge  prometheus.Gauge
	slaveConnsGauge   prometheus.Gauge
	sentOpsGauge      prometheus.Gauge
	receivedOpsGauge  prometheus.Gauge
	receivedDocsGauge prometheus.Gauge
	socketsAliveGauge prometheus.Gauge
	socketsInuseGauge prometheus.Gauge
	socketRefsGauge   prometheus.Gauge
}

// New returns a new Collector.
func New() *Collector {
	return &Collector{
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
		sentOpsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "sent_ops",
			Help:      "Current number of sent ops",
		}),
		receivedOpsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "received_ops",
			Help:      "Current number of received ops",
		}),
		receivedDocsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "mgo",
			Name:      "received_docs",
			Help:      "Current number of received docs",
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
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.clustersGauge.Describe(ch)
	c.masterConnsGauge.Describe(ch)
	c.slaveConnsGauge.Describe(ch)
	c.sentOpsGauge.Describe(ch)
	c.receivedOpsGauge.Describe(ch)
	c.receivedDocsGauge.Describe(ch)
	c.socketsAliveGauge.Describe(ch)
	c.socketsInuseGauge.Describe(ch)
	c.socketRefsGauge.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	stats := mgo.GetStats()
	c.clustersGauge.Set(float64(stats.Clusters))
	c.masterConnsGauge.Set(float64(stats.MasterConns))
	c.slaveConnsGauge.Set(float64(stats.SlaveConns))
	c.sentOpsGauge.Set(float64(stats.SentOps))
	c.receivedOpsGauge.Set(float64(stats.ReceivedOps))
	c.receivedDocsGauge.Set(float64(stats.ReceivedDocs))
	c.socketsAliveGauge.Set(float64(stats.SocketsAlive))
	c.socketsInuseGauge.Set(float64(stats.SocketsInUse))
	c.socketRefsGauge.Set(float64(stats.SocketRefs))

	c.clustersGauge.Collect(ch)
	c.masterConnsGauge.Collect(ch)
	c.slaveConnsGauge.Collect(ch)
	c.sentOpsGauge.Collect(ch)
	c.receivedOpsGauge.Collect(ch)
	c.receivedDocsGauge.Collect(ch)
	c.socketsAliveGauge.Collect(ch)
	c.socketsInuseGauge.Collect(ch)
	c.socketRefsGauge.Collect(ch)
}
