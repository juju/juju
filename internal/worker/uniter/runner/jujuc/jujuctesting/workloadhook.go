// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"
)

// WorkloadHook holds the values for the hook context.
type WorkloadHook struct {
	WorkloadName string
}

// Reset clears the WorkloadHook's data.
func (rh *WorkloadHook) Reset() {
	rh.WorkloadName = ""
}

// ContextWorkloadHook is a test double for jujuc.WorkloadHookContext.
type ContextWorkloadHook struct {
	contextBase
	info *WorkloadHook
}

// WorkloadName implements jujuc.WorkloadHookContext.
func (c *ContextWorkloadHook) WorkloadName() (string, error) {
	c.stub.AddCall("WorkloadName")
	err := c.stub.NextErr()
	if c.info.WorkloadName == "" {
		err = errors.NotFoundf("workload name")
	}

	return c.info.WorkloadName, err
}
