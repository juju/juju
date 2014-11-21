// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	corecharm "gopkg.in/juju/charm.v4"

	"github.com/juju/juju/worker/uniter/hook"
)

var logger = loggo.GetLogger("juju.worker.uniter.operation")

type Operation interface {
	String() string
	Prepare(state State) (*State, error)
	Execute(state State) (*State, error)
	Commit(state State) (*State, error)
}

var ErrSkipExecute = errors.New("operation already executed")
var ErrNeedsReboot = errors.New("reboot request issued")
var ErrHookFailed = errors.New("hook failed")

type stateChange struct {
	Kind     Kind
	Step     Step
	Hook     *hook.Info
	ActionId *string
	CharmURL *corecharm.URL
}

func (change stateChange) apply(state State) *State {
	state.Kind = change.Kind
	state.Step = change.Step
	state.Hook = change.Hook
	state.ActionId = change.ActionId
	state.CharmURL = change.CharmURL
	return &state
}
