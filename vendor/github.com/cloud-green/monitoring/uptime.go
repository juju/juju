// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package monitoring contains collectors that are used by cloud-green for monitoring our services.
package monitoring

import (
	"time"

	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const desc = "Unix epoch of server startup. Used to monitor uptime."

// UptimeCollector implements the prometheus.Collector interface and
// reports the unix timestamp of its start.
type UptimeCollector struct {
	desc   *prometheus.Desc
	metric prometheus.Metric
}

// Check implementation of prometheus.Collector interface.
var _ prometheus.Collector = (*UptimeCollector)(nil)

// NewUptimeCollector returns a new uptime collector with the specified properties.
// The provided time function will be used to report the uptime.
func NewUptimeCollector(namespace, subsystem, namePrefix string, t func() time.Time) (*UptimeCollector, error) {
	now := t().Unix()
	desc := prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, namePrefix), desc, nil, nil)
	m, err := prometheus.NewConstMetric(desc, prometheus.CounterValue, float64(now))
	if err != nil {
		return nil, errors.Annotate(err, "failed to create uptime metric")
	}
	return &UptimeCollector{
		desc:   desc,
		metric: m,
	}, nil
}

// Describe implements the prometheus.Collector interface.
func (u *UptimeCollector) Describe(c chan<- *prometheus.Desc) {
	c <- u.desc
}

// Collect implements the prometheus.Collector interface.
func (u *UptimeCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- u.metric
}
