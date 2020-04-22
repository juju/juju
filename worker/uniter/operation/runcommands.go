// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type runCommands struct {
	args         CommandArgs
	sendResponse CommandResponseFunc

	callbacks     Callbacks
	runnerFactory runner.Factory

	runner runner.Runner
	logger Logger

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
	rnr, err := rc.runnerFactory.NewCommandRunner(context.CommandInfo{
		RelationId:     rc.args.RelationId,
		RemoteUnitName: rc.args.RemoteUnitName,
		// TODO(jam): 2019-10-24 include RemoteAppName
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
	rc.logger.Tracef("run commands: %s", rc)
	if err := rc.callbacks.SetExecutingStatus("running commands"); err != nil {
		return nil, errors.Trace(err)
	}

	response, err := rc.runner.RunCommands(rc.args.Commands, rc.args.RunLocation)
	switch err {
	case context.ErrRequeueAndReboot:
		rc.logger.Warningf("cannot requeue external commands")
		fallthrough
	case context.ErrReboot:
		rc.sendResponse(response, nil)
		err = ErrNeedsReboot
	default:
		errorHandled := rc.sendResponse(response, err)
		if errorHandled {
			return nil, nil
		}
	}
	return nil, err
}

// Commit does nothing.
// Commit is part of the Operation interface.
func (rc *runCommands) Commit(state State) (*State, error) {
	return nil, nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (rc *runCommands) RemoteStateChanged(snapshot remotestate.Snapshot) {
}
