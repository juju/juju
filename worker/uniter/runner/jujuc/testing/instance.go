// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Instance holds the values for the hook context.
type Instance struct {
	AvailabilityZone string
	RebootPriority   *jujuc.RebootPriority
}

// ContextInstance is a test double for jujuc.ContextInstance.
type ContextInstance struct {
	contextBase
	info *Instance
}

// AvailabilityZone implements jujuc.ContextInstance.
func (c *ContextInstance) AvailabilityZone() (string, bool) {
	c.stub.AddCall("AvailabilityZone")
	c.stub.NextErr()

	return c.info.AvailabilityZone, true
}

// RequestReboot implements jujuc.ContextInstance.
func (c *ContextInstance) RequestReboot(priority jujuc.RebootPriority) error {
	c.stub.AddCall("RequestReboot", priority)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.RebootPriority = &priority
	return nil
}
