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
	Prepare(state State) (*StateChange, error)
	Execute(state State) (*StateChange, error)
	Commit(state State) (*StateChange, error)
}

var ErrSkipExecute = errors.New("operation already executed")
var ErrNeedsReboot = errors.New("reboot request issued")
var ErrHookFailed = errors.New("hook failed")

type StateChange struct {
	Kind     Kind
	Step     Step
	Hook     *hook.Info
	ActionId *string
	CharmURL *corecharm.URL
}
