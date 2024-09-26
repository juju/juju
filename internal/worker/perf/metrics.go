// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package perf

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace   = "juju"
	subsystemNamespace = "perf"
)

type Collector struct {
	iterationCount prometheus.Counter
}

func NewMetricsCollector() *Collector {
	// TODO (jam): we could use a CounterVec and do something like counts per model, but for now, we just
	//  want to know the total count
	return &Collector{
		iterationCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: subsystemNamespace,
			Name:      "iteration_count",
			Help:      "Count the number of iterations that this perf worker has completed",
		}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.iterationCount.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.iterationCount.Collect(ch)
}