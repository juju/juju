// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

// deploy implements charm install and charm upgrade operations.
type deploy struct {
	DoesNotRequireMachineLock

	kind     Kind
	charmURL string
	revert   bool
	resolved bool

	callbacks Callbacks
	deployer  charm.Deployer
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

// Prepare downloads and verifies the charm, and informs the controller
// that the unit will be using it. If the supplied state indicates that a
// hook was pending, that hook is recorded in the returned state.
// Prepare is part of the Operation interface.
func (d *deploy) Prepare(ctx context.Context, state State) (*State, error) {
	if err := d.checkAlreadyDone(state); err != nil {
		return nil, errors.Trace(err)
	}
	info, err := d.callbacks.GetArchiveInfo(d.charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := d.deployer.Stage(ctx, info); err != nil {
		return nil, errors.Trace(err)
	}
	// note: yes, this *should* be in Prepare, not Execute. Before we can safely
	// write out local state referencing the charm url (by returning the new
	// State to the Executor, below), we have to register our interest in that
	// charm on the controller. If we neglected to do so, the operation could
	// race with a new application-charm-url change on the controller, and lead to
	// failures on resume in which we try to obtain archive info for a charm that
	// has already been removed from the controller.
	if err := d.callbacks.SetCurrentCharm(ctx, d.charmURL); err != nil {
		return nil, errors.Trace(err)
	}
	return d.getState(state, Pending), nil
}

// Execute installs or upgrades the prepared charm, and preserves any hook
// recorded in the supplied state.
// Execute is part of the Operation interface.
func (d *deploy) Execute(ctx context.Context, state State) (*State, error) {
	if err := d.deployer.Deploy(); err == charm.ErrConflict {
		return nil, NewDeployConflictError(d.charmURL)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return d.getState(state, Done), nil
}

// Commit restores state for any interrupted hook, or queues an install or
// upgrade-charm hook if no hook was interrupted.
func (d *deploy) Commit(ctx context.Context, state State) (*State, error) {
	change := &stateChange{
		Kind: RunHook,
	}
	if hookInfo := d.interruptedHook(state); hookInfo != nil {
		change.Hook = hookInfo
		change.Step = Pending
		change.HookStep = state.HookStep
	} else {
		change.Hook = &hook.Info{Kind: deployHookKinds[d.kind]}
		change.Step = Queued
	}
	return change.apply(state), nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (d *deploy) RemoteStateChanged(snapshot remotestate.Snapshot) {
}

func (d *deploy) checkAlreadyDone(state State) error {
	if state.Kind != d.kind {
		return nil
	}
	if state.CharmURL != d.charmURL {
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
		HookStep: state.HookStep,
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
