// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

// Metrics holds the values for the hook sub-context.
type Metrics struct {
	Metrics []jujuc.Metric
}

// AddMetric adds a Metric for the provided data.
func (m *Metrics) AddMetric(key, value string, created time.Time) {
	m.Metrics = append(m.Metrics, jujuc.Metric{
		Key:   key,
		Value: value,
		Time:  created,
	})
}

// AddMetricLabels adds a Metric with labels for the provided data.
func (m *Metrics) AddMetricLabels(key, value string, created time.Time, labels map[string]string) {
	m.Metrics = append(m.Metrics, jujuc.Metric{
		Key:    key,
		Value:  value,
		Time:   created,
		Labels: labels,
	})
}

// ContextMetrics is a test double for jujuc.ContextMetrics.
type ContextMetrics struct {
	contextBase
	info *Metrics
}

// AddMetric implements jujuc.ContextMetrics.
func (c *ContextMetrics) AddMetric(key, value string, created time.Time) error {
	c.stub.AddCall("AddMetric", key, value, created)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.AddMetric(key, value, created)
	return nil
}

// AddMetricLabels implements jujuc.ContextMetrics.
func (c *ContextMetrics) AddMetricLabels(key, value string, created time.Time, labels map[string]string) error {
	c.stub.AddCall("AddMetricLabels", key, value, created, labels)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.AddMetricLabels(key, value, created, labels)
	return nil
}
