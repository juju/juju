// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/worker/uniter/context"
)

type CommandResponseFunc func(*utilexec.ExecResponse, error)

type runCommands struct {
	commands     string
	sendResponse CommandResponseFunc

	contextFactory context.Factory
	paths          context.Paths
	context        context.Context
	acquireLock    func(message string) (func(), error)
}

func (rc *runCommands) String() string {
	return "run commands"
}

func (rc *runCommands) Prepare(state State) (*StateChange, error) {
	ctx, err := rc.contextFactory.NewRunContext()
	if err != nil {
		return nil, err
	}
	rc.context = ctx
	// Commands only make sense at runtime; this is totally ephemeral; no
	// state change at all.
	return nil, nil
}

func (rc *runCommands) Execute(state State) (*StateChange, error) {
	unlock, err := rc.acquireLock("run commands")
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

func (rc *runCommands) Commit(state State) (*StateChange, error) {
	// Commands only make sense at runtime; this is totally ephemeral; no
	// state change at all.
	return nil, nil
}
