// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statemetrics

import "github.com/prometheus/client_golang/prometheus"

// In order to be able to use the prometheus testutil library, we need
// access to the actual GaugeVec values.

// ModelsGauge returns the internal gauge for testing.
func (c *Collector) ModelsGauge() *prometheus.GaugeVec {
	return c.models
}

// MachinesGauge returns the internal gauge for testing.
func (c *Collector) MachinesGauge() *prometheus.GaugeVec {
	return c.machines
}

// UsersGauge returns the internal gauge for testing.
func (c *Collector) UsersGauge() *prometheus.GaugeVec {
	return c.users
}

// ScrapeDuration returns the internal gauge for testing.
func (c *Collector) ScrapeDurationGauge() prometheus.Gauge {
	return c.scrapeDuration
}

// ScrapeErrors returns the internal gauge for testing.
func (c *Collector) ScrapeErrorsGauge() prometheus.Gauge {
	return c.scrapeErrors
}
