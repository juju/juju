// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
)

// ContextMetrics is a test double for jujuc.ContextMetrics.
type ContextMetrics struct {
	Stub *testing.Stub
}

func (c *ContextMetrics) init() {
	if c.Stub == nil {
		c.Stub = &testing.Stub{}
	}
}

// AddMetric implements jujuc.ContextMetrics.
func (c *ContextMetrics) AddMetric(key, value string, created time.Time) error {
	c.Stub.AddCall("AddMetric", key, value, created)
	err := c.Stub.NextErr()
	c.init()
	return errors.Trace(err)
}
