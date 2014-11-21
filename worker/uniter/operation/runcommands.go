// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/juju/worker/uniter/context"
)

type runCommands struct {
	commands     string
	sendResponse CommandResponseFunc

	paths          context.Paths
	callbacks      Callbacks
	contextFactory context.Factory

	context context.Context
}

func (rc *runCommands) String() string {
	return "run commands"
}

func (rc *runCommands) Prepare(state State) (*State, error) {
	ctx, err := rc.contextFactory.NewRunContext()
	if err != nil {
		return nil, err
	}
	rc.context = ctx
	// Commands only make sense at runtime; this is totally ephemeral; no
	// state change at all.
	// TODO(fwereade): we *should* handle interrupted actions, and make sure
	// they;re marked as failed, but that's not for now.
	return nil, nil
}

func (rc *runCommands) Execute(state State) (*State, error) {
	unlock, err := rc.callbacks.AcquireExecutionLock("run commands")
	if err != nil {
		return nil, err
	}
	defer unlock()

	runner := context.NewRunner(rc.context, rc.paths)
	response, err := runner.RunCommands(rc.commands)
	switch err {
	case context.ErrRequeueAndReboot:
		logger.Warningf("cannot requeue external commands")
		fallthrough
	case context.ErrReboot:
		err = ErrNeedsReboot
	}
	rc.sendResponse(response, err)
	// Commands only make sense at runtime; this is totally ephemeral; no
	// state change at all.
	return nil, nil
}

func (rc *runCommands) Commit(state State) (*State, error) {
	// Commands only make sense at runtime; this is totally ephemeral; no
	// state change at all.
	return nil, nil
}
