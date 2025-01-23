// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/leadership"
)

var (
	errIsMinion = errors.New("not the leader")
)

// LeadershipContext provides several hooks.Context methods. It
// exists separately of HookContext for clarity, and ease of testing.
type LeadershipContext interface {
	IsLeader() (bool, error)
}

type leadershipContext struct {
	tracker  leadership.Tracker
	isMinion bool
}

// NewLeadershipContext creates a leadership context for the specified unit.
func NewLeadershipContext(tracker leadership.Tracker) LeadershipContext {
	return &leadershipContext{
		tracker: tracker,
	}
}

// IsLeader is part of the hooks.Context interface.
func (c *leadershipContext) IsLeader() (bool, error) {
	// This doesn't technically need an error return, but that feels like a
	// happy accident of the current implementation and not a reason to change
	// the interface we're implementing.
	err := c.ensureLeader()
	switch err {
	case nil:
		return true, nil
	case errIsMinion:
		return false, nil
	}
	return false, errors.Trace(err)
}

func (c *leadershipContext) ensureLeader() error {
	if c.isMinion {
		return errIsMinion
	}
	success := c.tracker.ClaimLeader().Wait()
	if !success {
		c.isMinion = true
		return errIsMinion
	}
	return nil
}
