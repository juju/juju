// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
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
