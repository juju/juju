// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"
)

// Leadership holds the values for the hook context.
type Leadership struct {
	IsLeader bool
}

// ContextLeader is a test double for jujuc.ContextLeader.
type ContextLeader struct {
	contextBase
	info *Leadership
}

// IsLeader implements jujuc.ContextLeader.
func (c *ContextLeader) IsLeader() (bool, error) {
	c.stub.AddCall("IsLeader")
	if err := c.stub.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	return c.info.IsLeader, nil
}
