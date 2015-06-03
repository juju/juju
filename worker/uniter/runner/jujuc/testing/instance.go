// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Instance holds the values for the hook context.
type Instance struct {
	AvailabilityZone string
}

// ContextInstance is a test double for jujuc.ContextInstance.
type ContextInstance struct {
	Stub *testing.Stub
	Info *Instance
}

// AvailabilityZone implements jujuc.ContextInstance.
func (ci *ContextInstance) AvailabilityZone() (string, bool) {
	ci.Stub.AddCall("AvailabilityZone")
	ci.Stub.NextErr()
	return ci.Info.AvailabilityZone, true
}

// RequestReboot implements jujuc.ContextInstance.
func (ci *ContextInstance) RequestReboot(priority jujuc.RebootPriority) error {
	ci.Stub.AddCall("RequestReboot", priority)
	return errors.Trace(ci.Stub.NextErr())
}
