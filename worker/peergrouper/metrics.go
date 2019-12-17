// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"
	"sync"

	"github.com/juju/replicaset"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "juju_peergrouper"

	idLabel      = "id"
	addressLabel = "address"
	stateLabel   = "state"
)

var (
	replicasetLabelNames = []string{
		idLabel,
		addressLabel,
		stateLabel,
	}
)

// Collector is a prometheus.Collector that collects metrics about
// the Juju global state.
type Collector struct {
	replicasetStatus prometheus.Gauge

	mu     sync.Mutex
	status []replicaset.MemberStatus
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		replicasetStatus: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "replicaset_status",
				Help:      "The details of the mongo replicaset.",
			},
		),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.replicasetStatus.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.replicasetStatus.Reset()

	c.mu.Lock()
	for _, member := range c.status {
		c.replicasetStatus.With(prometheus.Labels{
			idLabel:      fmt.Sprint(member.Id),
			addressLabel: member.Address,
			stateLabel:   member.State.String(),
		}).Inc()
	}
	c.mu.Unlock()

	c.replicasetStatus.Collect(ch)
}

func (c *Collector) update(statuses map[string]replicaset.MemberStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.status = make([]replicaset.MemberStatus, 0, len(statuses))
	for _, status := range statuses {
		c.status = append(c.status, status)
	}
}
