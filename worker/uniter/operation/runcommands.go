// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/runner"
)

type runCommands struct {
	args         CommandArgs
	sendResponse CommandResponseFunc

	callbacks     Callbacks
	runnerFactory runner.Factory

	runner runner.Runner

	RequiresMachineLock
}

// String is part of the Operation interface.
func (rc *runCommands) String() string {
	suffix := ""
	if rc.args.RelationId != -1 {
		infix := ""
		if rc.args.RemoteUnitName != "" {
			infix = "; " + rc.args.RemoteUnitName
		}
		suffix = fmt.Sprintf(" (%d%s)", rc.args.RelationId, infix)
	}
	return "run commands" + suffix
}

// Prepare ensures the commands can be run. It never returns a state change.
// Prepare is part of the Operation interface.
func (rc *runCommands) Prepare(state State) (*State, error) {
	rnr, err := rc.runnerFactory.NewCommandRunner(runner.CommandInfo{
		RelationId:      rc.args.RelationId,
		RemoteUnitName:  rc.args.RemoteUnitName,
		ForceRemoteUnit: rc.args.ForceRemoteUnit,
	})
	if err != nil {
		return nil, err
	}
	err = rnr.Context().Prepare()
	if err != nil {
		return nil, errors.Trace(err)
	}
	rc.runner = rnr

	return nil, nil
}

// Execute runs the commands and dispatches their results. It never returns a
// state change.
// Execute is part of the Operation interface.
func (rc *runCommands) Execute(state State) (*State, error) {
	if err := rc.callbacks.SetExecutingStatus("running commands"); err != nil {
		return nil, errors.Trace(err)
	}

	response, err := rc.runner.RunCommands(rc.args.Commands)
	switch err {
	case runner.ErrRequeueAndReboot:
		logger.Warningf("cannot requeue external commands")
		fallthrough
	case runner.ErrReboot:
		err = ErrNeedsReboot
	}
	rc.sendResponse(response, err)
	return nil, nil
}

// Commit does nothing.
// Commit is part of the Operation interface.
func (rc *runCommands) Commit(state State) (*State, error) {
	return nil, nil
}
