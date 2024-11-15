// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	utilexec "github.com/juju/utils/v4/exec"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/interface_mock.go github.com/juju/juju/internal/worker/uniter/operation Operation,Factory,Callbacks

// Operation encapsulates the stages of the various things the uniter can do,
// and the state changes that need to be recorded as they happen. Operations
// are designed to be Run (or Skipped) by an Executor, which supplies starting
// state and records the changes returned.
type Operation interface {

	// String returns a short representation of the operation.
	String() string

	// NeedsGlobalMachineLock returns a bool expressing whether we need to lock the machine.
	NeedsGlobalMachineLock() bool

	// ExecutionGroup returns a string used to construct the name of the machine lock.
	ExecutionGroup() string

	// Prepare ensures that the operation is valid and ready to be executed.
	// If it returns a non-nil state, that state will be validated and recorded.
	// If it returns ErrSkipExecute, it indicates that the operation can be
	// committed directly.
	Prepare(ctx stdcontext.Context, state State) (*State, error)

	// Execute carries out the operation. It must not be called without having
	// called Prepare first. If it returns a non-nil state, that state will be
	// validated and recorded.
	Execute(ctx stdcontext.Context, state State) (*State, error)

	// Commit ensures that the operation's completion is recorded. If it returns
	// a non-nil state, that state will be validated and recorded.
	Commit(ctx stdcontext.Context, state State) (*State, error)

	// RemoteStateChanged is called when the remote state changed during execution
	// of the operation.
	RemoteStateChanged(ctx stdcontext.Context, snapshot remotestate.Snapshot)
}

// WrappedOperation extends Operation to provide access to the wrapped operation.
type WrappedOperation interface {
	Operation

	WrappedOperation() Operation
}

// Unwrap peels back one layer of a wrapped operation.
func Unwrap(op Operation) Operation {
	if op == nil {
		return nil
	}
	if wrapped, ok := op.(WrappedOperation); ok {
		return wrapped.WrappedOperation()
	}
	return op
}

// Executor records and exposes uniter state, and applies suitable changes as
// operations are run or skipped.
type Executor interface {

	// State returns a copy of the executor's current operation state.
	State() State

	// Run will Prepare, Execute, and Commit the supplied operation, writing
	// indicated state changes between steps. If any step returns an unknown
	// error, the run will be aborted and an error will be returned.
	// On remote state change, the executor will fire the operation's
	// RemoteStateChanged method.
	Run(stdcontext.Context, Operation, <-chan remotestate.Snapshot) error

	// Skip will Commit the supplied operation, and write any state change
	// indicated. If Commit returns an error, so will Skip.
	Skip(stdcontext.Context, Operation) error
}

// Factory creates operations.
type Factory interface {

	// NewInstall creates an install operation for the supplied charm.
	NewInstall(charmURL string) (Operation, error)

	// NewUpgrade creates an upgrade operation for the supplied charm.
	NewUpgrade(charmURL string) (Operation, error)

	// NewRevertUpgrade creates an operation to clear the unit's resolved flag,
	// and execute an upgrade to the supplied charm that is careful to excise
	// remnants of a previously failed upgrade to a different charm.
	NewRevertUpgrade(charmURL string) (Operation, error)

	// NewResolvedUpgrade creates an operation to clear the unit's resolved flag,
	// and execute an upgrade to the supplied charm that is careful to preserve
	// non-overlapping remnants of a previously failed upgrade to the same charm.
	NewResolvedUpgrade(charmURL string) (Operation, error)

	// NewRunHook creates an operation to execute the supplied hook.
	NewRunHook(hookInfo hook.Info) (Operation, error)

	// NewSkipHook creates an operation to mark the supplied hook as
	// completed successfully, without executing the hook.
	NewSkipHook(hookInfo hook.Info) (Operation, error)

	// NewAction creates an operation to execute the supplied action.
	NewAction(ctx stdcontext.Context, actionId string) (Operation, error)

	// NewFailAction creates an operation that marks an action as failed.
	NewFailAction(actionId string) (Operation, error)

	// NewCommands creates an operation to execute the supplied script in the
	// indicated relation context, and pass the results back over the supplied
	// func.
	NewCommands(args CommandArgs, sendResponse CommandResponseFunc) (Operation, error)

	// NewAcceptLeadership creates an operation to ensure the uniter acts as
	// application leader.
	NewAcceptLeadership() (Operation, error)

	// NewResignLeadership creates an operation to ensure the uniter does not
	// act as application leader.
	NewResignLeadership() (Operation, error)

	// NewNoOpSecretsRemoved creates an operation to update the secrets
	// state when secrets are removed.
	NewNoOpSecretsRemoved(uris []string) (Operation, error)
}

// CommandArgs stores the arguments for a Command operation.
type CommandArgs struct {
	// Commands is the arbitrary commands to execute on the unit
	Commands string
	// RelationId is the relation context to execute the commands in.
	RelationId int
	// RemoteUnitName is the remote unit for the relation context.
	RemoteUnitName string
	// TODO(jam): 2019-10-24 Include RemoteAppName
	// ForceRemoteUnit skips unit inference and existence validation.
	ForceRemoteUnit bool
}

// Validate the command arguments.
func (args CommandArgs) Validate() error {
	if args.Commands == "" {
		return errors.New("commands required")
	}
	if args.RemoteUnitName != "" {
		if args.RelationId == -1 {
			return errors.New("remote unit not valid without relation")
		} else if !names.IsValidUnit(args.RemoteUnitName) {
			return errors.Errorf("invalid remote unit name %q", args.RemoteUnitName)
		}
	}
	return nil
}

// CommandResponseFunc is for marshalling command responses back to the source
// of the original request.
type CommandResponseFunc func(*utilexec.ExecResponse, error) bool

// Callbacks exposes all the uniter code that's required by the various operations.
// It's far from cohesive, and fundamentally represents inappropriate coupling, so
// it's a prime candidate for future refactoring.
type Callbacks interface {
	// PrepareHook and CommitHook exist so that we can defer worrying about how
	// to untangle Uniter.relationers from everything else. They're only used by
	// RunHook operations.
	PrepareHook(ctx stdcontext.Context, info hook.Info) (name string, err error)
	CommitHook(ctx stdcontext.Context, info hook.Info) error

	// SetExecutingStatus sets the agent state to "Executing" with a message.
	SetExecutingStatus(stdcontext.Context, string) error

	// NotifyHook* exist so that we can defer worrying about how to untangle the
	// callbacks inserted for uniter_test. They're only used by RunHook operations.

	NotifyHookCompleted(string, context.Context)
	NotifyHookFailed(string, context.Context)

	// The following methods exist primarily to allow us to test operation code
	// without using a live api connection.

	// FailAction marks the supplied action failed. It's only used by
	// RunActions operations.
	FailAction(ctx stdcontext.Context, actionId, message string) error

	// ActionStatus returns the status of the action required by the action operation for
	// cancelation.
	ActionStatus(ctx stdcontext.Context, actionId string) (string, error)

	// GetArchiveInfo is used to find out how to download a charm archive. It's
	// only used by Deploy operations.
	GetArchiveInfo(charmURL string) (charm.BundleInfo, error)

	// SetCurrentCharm records intent to deploy a given charm. It must be called
	// *before* recording local state referencing that charm, to ensure there's
	// no path by which the controller can legitimately garbage collect that
	// charm or the application's settings for it. It's only used by Deploy operations.
	SetCurrentCharm(ctx stdcontext.Context, charmURL string) error

	// SetSecretRotated updates the secret rotation status.
	SetSecretRotated(ctx stdcontext.Context, url string, originalRevision int) error

	// SecretsRemoved updates the unit secret state when
	// secrets are removed.
	SecretsRemoved(ctx stdcontext.Context, uris []string) error
}

// StorageUpdater is an interface used for updating local knowledge of storage
// attachments.
type StorageUpdater interface {
	// UpdateStorage updates local knowledge of the storage attachments
	// with the specified tags.
	UpdateStorage([]names.StorageTag) error
}

// ActionGetter provides a method to query a given action.
type ActionGetter interface {
	Action(ctx stdcontext.Context, tag names.ActionTag) (*uniter.Action, error)
}
