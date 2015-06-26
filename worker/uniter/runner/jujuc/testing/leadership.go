// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
)

// Leadership holds the values for the hook context.
type Leadership struct {
	IsLeader       bool
	LeaderSettings map[string]string
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

// LeaderSettings implements jujuc.ContextLeader.
func (c *ContextLeader) LeaderSettings() (map[string]string, error) {
	c.stub.AddCall("LeaderSettings")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return c.info.LeaderSettings, nil
}

// WriteLeaderSettings implements jujuc.ContextLeader.
func (c *ContextLeader) WriteLeaderSettings(settings map[string]string) error {
	c.stub.AddCall("WriteLeaderSettings")
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.LeaderSettings = settings
	return nil
}
