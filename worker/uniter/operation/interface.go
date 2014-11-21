// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
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

type Factory interface {
	NewDeploy(charmURL *corecharm.URL, kind Kind) (Operation, error)
	NewHook(hookInfo hook.Info) (Operation, error)
	NewAction(actionId string) (Operation, error)
	NewCommands(commands string, sendResponse CommandResponseFunc) (Operation, error)
}

type Executor interface {
	State() State
	Run(Operation) error
	Skip(Operation) error
}
