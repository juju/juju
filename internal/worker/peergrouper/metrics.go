// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"sync"

	"github.com/juju/replicaset/v3"
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
// the mongo replicaset status.
type Collector struct {
	replicasetStatus *prometheus.GaugeVec

	mu     sync.Mutex
	status []replicaset.MemberStatus
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		replicasetStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "replicaset_status",
				Help:      "The details of the mongo replicaset.",
			},
			replicasetLabelNames,
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

func (c *Collector) report() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	peers := make(map[string]interface{})
	for _, member := range c.status {
		peers[fmt.Sprint(member.Id)] = map[string]interface{}{
			"address": member.Address,
			"state":   member.State.String(),
		}
	}
	return map[string]interface{}{
		"replicaset": peers,
	}
}
