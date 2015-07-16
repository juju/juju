// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	corecharm "gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
)

// deploy implements charm install and charm upgrade operations.
type deploy struct {
	DoesNotRequireMachineLock

	kind     Kind
	charmURL *corecharm.URL
	revert   bool
	resolved bool

	callbacks Callbacks
	deployer  charm.Deployer
	abort     <-chan struct{}
}

// String is part of the Operation interface.
func (d *deploy) String() string {
	verb := "upgrade to"
	prefix := ""
	switch {
	case d.kind == Install:
		verb = "install"
	case d.revert:
		prefix = "switch "
	case d.resolved:
		prefix = "continue "
	}
	return fmt.Sprintf("%s%s %s", prefix, verb, d.charmURL)
}

// Prepare downloads and verifies the charm, and informs the state server
// that the unit will be using it. If the supplied state indicates that a
// hook was pending, that hook is recorded in the returned state.
// Prepare is part of the Operation interface.
func (d *deploy) Prepare(state State) (*State, error) {
	if err := d.checkAlreadyDone(state); err != nil {
		return nil, errors.Trace(err)
	}
	if d.revert {
		if err := d.deployer.NotifyRevert(); err != nil {
			return nil, errors.Trace(err)
		}
	}
	if d.resolved {
		if err := d.deployer.NotifyResolved(); err != nil {
			return nil, errors.Trace(err)
		}
	}
	info, err := d.callbacks.GetArchiveInfo(d.charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := d.deployer.Stage(info, d.abort); err != nil {
		return nil, errors.Trace(err)
	}
	// note: yes, this *should* be in Prepare, not Execute. Before we can safely
	// write out local state referencing the charm url (by returning the new
	// State to the Executor, below), we have to register our interest in that
	// charm on the state server. If we neglected to do so, the operation could
	// race with a new service-charm-url change on the state server, and lead to
	// failures on resume in which we try to obtain archive info for a charm that
	// has already been removed from the state server.
	if err := d.callbacks.SetCurrentCharm(d.charmURL); err != nil {
		return nil, errors.Trace(err)
	}
	return d.getState(state, Pending), nil
}

// Execute installs or upgrades the prepared charm, and preserves any hook
// recorded in the supplied state.
// Execute is part of the Operation interface.
func (d *deploy) Execute(state State) (*State, error) {
	if err := d.deployer.Deploy(); err == charm.ErrConflict {
		return nil, NewDeployConflictError(d.charmURL)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return d.getState(state, Done), nil
}

// Commit restores state for any interrupted hook, or queues an install or
// upgrade-charm hook if no hook was interrupted.
func (d *deploy) Commit(state State) (*State, error) {
	if err := d.callbacks.InitializeMetricsTimers(); err != nil {
		return nil, errors.Trace(err)
	}
	change := &stateChange{
		Kind: RunHook,
	}
	if hookInfo := d.interruptedHook(state); hookInfo != nil {
		change.Hook = hookInfo
		change.Step = Pending
	} else {
		change.Hook = &hook.Info{Kind: deployHookKinds[d.kind]}
		change.Step = Queued
	}
	return change.apply(state), nil
}

func (d *deploy) checkAlreadyDone(state State) error {
	if state.Kind != d.kind {
		return nil
	}
	if *state.CharmURL != *d.charmURL {
		return nil
	}
	if state.Step == Done {
		return ErrSkipExecute
	}
	return nil
}

func (d *deploy) getState(state State, step Step) *State {
	return stateChange{
		Kind:     d.kind,
		Step:     step,
		CharmURL: d.charmURL,
		Hook:     d.interruptedHook(state),
	}.apply(state)
}

func (d *deploy) interruptedHook(state State) *hook.Info {
	switch state.Kind {
	case RunHook, Upgrade:
		return state.Hook
	}
	return nil
}

// deployHookKinds determines what kind of hook should be queued after a
// given deployment operation.
var deployHookKinds = map[Kind]hooks.Kind{
	Install: hooks.Install,
	Upgrade: hooks.UpgradeCharm,
}
