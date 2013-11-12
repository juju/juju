// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// Environment represents a juju environment as seen by a machiner worker.
type Environment struct {
	tag  string
	life params.Life
	st   *State
}

// Tag returns the environment's tag.
func (e *Environment) Tag() string {
	return e.tag
}

// Life returns the environment's lifecycle value.
func (e *Environment) Life() params.Life {
	return e.life
}

// Refresh updates the cached local copy of the environment's data.
func (e *Environment) Refresh() error {
	life, err := e.st.entityLife(e.tag)
	if err != nil {
		return err
	}
	e.life = life
	return nil
}

// Watch returns a watcher for observing changes to the machine.
func (e *Environment) Watch() (watcher.NotifyWatcher, error) {
	return e.st.watch(e.tag)
}
