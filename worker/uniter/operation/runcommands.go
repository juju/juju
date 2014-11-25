// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/juju/worker/uniter/context"
)

type runCommands struct {
	commands       string
	relationId     int
	remoteUnitName string
	sendResponse   CommandResponseFunc

	callbacks      Callbacks
	contextFactory context.Factory

	context context.Context
}

func (rc *runCommands) String() string {
	suffix := ""
	if rc.relationId != -1 {
		infix := ""
		if rc.remoteUnitName != "" {
			infix = "; " + rc.remoteUnitName
		}
		suffix = fmt.Sprintf(" (%d%s)", rc.relationId, infix)
	}
	return "run commands" + suffix
}

func (rc *runCommands) Prepare(state State) (*State, error) {
	ctx, err := rc.contextFactory.NewRunContext(rc.relationId, rc.remoteUnitName)
	if err != nil {
		return nil, err
	}
	rc.context = ctx
	// Commands only make sense at runtime; this is totally ephemeral; no
	// state change at all.
	return nil, nil
}

func (rc *runCommands) Execute(state State) (*State, error) {
	unlock, err := rc.callbacks.AcquireExecutionLock("run commands")
	if err != nil {
		return nil, err
	}
	defer unlock()

	runner := rc.callbacks.GetRunner(rc.context)
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
