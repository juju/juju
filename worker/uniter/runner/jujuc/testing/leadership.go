// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
)

// Leadership holds the values for the hook context.
type Leadership struct {
	IsLeader       bool
	LeaderSettings map[string]string
}

// ContextLeader is a test double for jujuc.ContextLeader.
type ContextLeader struct {
	Stub *testing.Stub
	Info *Leadership
}

// IsLeader implements jujuc.ContextLeader.
func (c *ContextLeader) IsLeader() (bool, error) {
	c.Stub.AddCall("IsLeader")
	if err := c.Stub.NextErr(); err != nil {
		return false, errors.Trace(err)
	}
	return c.Info.IsLeader, nil
}

// LeaderSettings implements jujuc.ContextLeader.
func (c *ContextLeader) LeaderSettings() (map[string]string, error) {
	c.Stub.AddCall("LeaderSettings")
	if err := c.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.Info.LeaderSettings, nil
}

// WriteLeaderSettings implements jujuc.ContextLeader.
func (c *ContextLeader) WriteLeaderSettings(settings map[string]string) error {
	c.Stub.AddCall("WriteLeaderSettings")
	if err := c.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}
	c.Info.LeaderSettings = settings
	return nil
}
