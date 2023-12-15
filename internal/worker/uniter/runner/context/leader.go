// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/leadership"
)

var (
	errIsMinion = errors.New("not the leader")
)

// LeadershipContext provides several hooks.Context methods. It
// exists separately of HookContext for clarity, and ease of testing.
type LeadershipContext interface {
	IsLeader() (bool, error)
	LeaderSettings() (map[string]string, error)
	WriteLeaderSettings(map[string]string) error
}

type leadershipContext struct {
	accessor        uniter.LeadershipSettingsAccessor
	tracker         leadership.Tracker
	applicationName string
	unitName        string

	isMinion bool
	settings map[string]string
}

// NewLeadershipContext creates a leadership context for the specified unit.
func NewLeadershipContext(accessor uniter.LeadershipSettingsAccessor, tracker leadership.Tracker, unitName string) LeadershipContext {
	return &leadershipContext{
		accessor:        accessor,
		tracker:         tracker,
		applicationName: tracker.ApplicationName(),
		unitName:        unitName,
	}
}

// IsLeader is part of the hooks.Context interface.
func (ctx *leadershipContext) IsLeader() (bool, error) {
	// This doesn't technically need an error return, but that feels like a
	// happy accident of the current implementation and not a reason to change
	// the interface we're implementing.
	err := ctx.ensureLeader()
	switch err {
	case nil:
		return true, nil
	case errIsMinion:
		return false, nil
	}
	return false, errors.Trace(err)
}

// WriteLeaderSettings is part of the hooks.Context interface.
func (ctx *leadershipContext) WriteLeaderSettings(settings map[string]string) error {
	// This may trigger a lease refresh; it would be desirable to use a less
	// eager approach here, but we're working around a race described in
	// `apiserver/leadership.LeadershipSettingsAccessor.Merge`, and as of
	// 2015-02-19 it's better to stay eager.
	err := ctx.ensureLeader()
	if err == nil {
		// Clear local settings; if we need them again we should use the values
		// as merged by the server. But we don't need to get them again right now;
		// the charm may not need to ask again before the hook finishes.
		ctx.settings = nil
		err = ctx.accessor.Merge(ctx.applicationName, ctx.unitName, settings)
	}
	return errors.Annotate(err, "cannot write settings")
}

// LeaderSettings is part of the hooks.Context interface.
func (ctx *leadershipContext) LeaderSettings() (map[string]string, error) {
	if ctx.settings == nil {
		var err error
		ctx.settings, err = ctx.accessor.Read(ctx.applicationName)
		if err != nil {
			return nil, errors.Annotate(err, "cannot read settings")
		}
	}
	result := map[string]string{}
	for key, value := range ctx.settings {
		result[key] = value
	}
	return result, nil
}

func (ctx *leadershipContext) ensureLeader() error {
	if ctx.isMinion {
		return errIsMinion
	}
	success := ctx.tracker.ClaimLeader().Wait()
	if !success {
		ctx.isMinion = true
		return errIsMinion
	}
	return nil
}
