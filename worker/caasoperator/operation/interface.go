// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE store for details.

package operation

import (
	"github.com/juju/loggo"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/runner"
)

var logger = loggo.GetLogger("juju.worker.caasoperator.operation")

// Operation encapsulates the stages of the various things the operatoran do,
// and the state changes that need to be recorded as they happen. Operations
// are designed to be Run (or Skipped) by an Executor, which supplies starting
// state and records the changes returned.
type Operation interface {

	// String returns a short representation of the operation.
	String() string

	// Prepare ensures that the operation is valid and ready to be executed.
	// If it returns a non-nil state, that state will be validated and recorded.
	// If it returns ErrSkipExecute, it indicates that the operation can be
	// committed directly.
	Prepare(state State) (*State, error)

	// Execute carries out the operation. It must not be called without having
	// called Prepare first. If it returns a non-nil state, that state will be
	// validated and recorded.
	Execute(state State) (*State, error)

	// Commit ensures that the operation's completion is recorded. If it returns
	// a non-nil state, that state will be validated and recorded.
	Commit(state State) (*State, error)
}

// Executor records and exposes operator state, and applies suitable changes as
// operations are run or skipped.
type Executor interface {

	// State returns a copy of the executor's current operation state.
	State() State

	// Run will Prepare, Execute, and Commit the supplied operation, writing
	// indicated state changes between steps. If any step returns an unknown
	// error, the run will be aborted and an error will be returned.
	Run(Operation) error

	// Skip will Commit the supplied operation, and write any state change
	// indicated. If Commit returns an error, so will Skip.
	Skip(Operation) error
}

// Factory creates operations.
type Factory interface {
	// NewRunHook creates an operation to execute the supplied hook.
	NewRunHook(hookInfo hook.Info) (Operation, error)

	// NewSkipHook creates an operation to mark the supplied hook as
	// completed successfully, without executing the hook.
	NewSkipHook(hookInfo hook.Info) (Operation, error)
}

// CommandArgs stores the arguments for a Command operation.
type CommandArgs struct {
	// Commands is the arbitrary commands to execute on the unit
	Commands string
	// RelationId is the relation context to execute the commands in.
	RelationId int
	// RemoteUnitName is the remote unit for the relation context.
	RemoteUnitName string
	// ForceRemoteUnit skips unit inference and existence validation.
	ForceRemoteUnit bool
}

// CommandResponseFunc is for marshalling command responses back to the source
// of the original request.
type CommandResponseFunc func(*utilexec.ExecResponse, error)

// Callbacks exposes all the operator code that's required by the various operations.
type Callbacks interface {
	PrepareHook(info hook.Info) (name string, err error)
	CommitHook(info hook.Info) error

	// SetExecutingStatus sets the agent state to "Executing" with a message.
	SetExecutingStatus(string) error

	// NotifyHook* exist so that we can defer worrying about how to untangle the
	// callbacks inserted for caasoperator_test. They're only used by RunHook operations.
	NotifyHookCompleted(string, runner.Context)
	NotifyHookFailed(string, runner.Context)
}
