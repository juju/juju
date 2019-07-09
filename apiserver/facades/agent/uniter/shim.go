// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/state"
	"gopkg.in/juju/charm.v6"
)

// unit is an indirection for operations common to state and cache.
type unit interface {
	ConfigSettings() (charm.Settings, error)

	// TODO (manadart 2017-07-08) This should probably return
	// core.StringsWatcher, but that does not implement Resource
	// or Errer, required by API-handled watchers.
	WatchConfigSettingsHash() (state.StringsWatcher, error)
}

type cacheUnit struct {
	*cache.Unit
}

// WatchConfigSettingsHash wraps the call to a cache-based config watch
// so that it is congruent with the state watcher.
func (u cacheUnit) WatchConfigSettingsHash() (state.StringsWatcher, error) {
	w, err := u.Unit.WatchConfigSettings()
	return w, errors.Trace(err)
}
