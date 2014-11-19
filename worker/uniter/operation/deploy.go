// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	corecharm "gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
)

type deploy struct {
	kind     Kind
	charmURL *corecharm.URL
	getInfo  func(*corecharm.URL) (charm.BundleInfo, error)
	setCharm func(*corecharm.URL) error
	deployer charm.Deployer
	abort    <-chan struct{}
}

func (d *deploy) String() string {
	return fmt.Sprintf("%s %s", d.kind, d.charmURL)
}

func (d *deploy) Prepare(state State) (*StateChange, error) {
	if err := d.checkAlreadyDone(state); err != nil {
		return nil, err
	}
	info, err := d.getInfo(d.charmURL)
	if err != nil {
		return nil, err
	}
	if err := d.deployer.Stage(info, d.abort); err != nil {
		return nil, err
	}
	if err := d.setCharm(d.charmURL); err != nil {
		return nil, err
	}
	return d.getChange(state, Pending), nil
}

func (d *deploy) Execute(state State) (*StateChange, error) {
	if err := d.deployer.Deploy(); err != nil {
		return nil, err
	}
	return d.getChange(state, Done), nil
}

func (d *deploy) Commit(state State) (*StateChange, error) {
	change := &StateChange{
		Kind: RunHook,
	}
	if hookInfo := d.interruptedHook(state); hookInfo != nil {
		change.Hook = hookInfo
		change.Step = Pending
		return change, nil
	}
	change.Hook = &hook.Info{Kind: deployHookKinds[d.kind]}
	change.Step = Queued
	return change, nil
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

func (d *deploy) getChange(state State, step Step) *StateChange {
	return &StateChange{
		Kind:     d.kind,
		Step:     step,
		CharmURL: d.charmURL,
		Hook:     d.interruptedHook(state),
	}
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
