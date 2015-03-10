// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/loggo"
	"github.com/juju/names"
	utilexec "github.com/juju/utils/exec"
	corecharm "gopkg.in/juju/charm.v4"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
)

var logger = loggo.GetLogger("juju.worker.uniter.operation")

// Operation encapsulates the stages of the various things the uniter can do,
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

// Executor records and exposes uniter state, and applies suitable changes as
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

	// NewInstall creates an install operation for the supplied charm.
	NewInstall(charmURL *corecharm.URL) (Operation, error)

	// NewUpgrade creates an upgrade operation for the supplied charm.
	NewUpgrade(charmURL *corecharm.URL) (Operation, error)

	// NewRevertUpgrade creates an operation to clear the unit's resolved flag,
	// and execute an upgrade to the supplied charm that is careful to excise
	// remnants of a previously failed upgrade to a different charm.
	NewRevertUpgrade(charmURL *corecharm.URL) (Operation, error)

	// NewResolvedUpgrade creates an operation to clear the unit's resolved flag,
	// and execute an upgrade to the supplied charm that is careful to preserve
	// non-overlapping remnants of a previously failed upgrade to the same charm.
	NewResolvedUpgrade(charmURL *corecharm.URL) (Operation, error)

	// NewRunHook creates an operation to execute the supplied hook.
	NewRunHook(hookInfo hook.Info) (Operation, error)

	// NewRetryHook creates an operation to clear the unit's resolved flag, and
	// re-execute the supplied hook.
	NewRetryHook(hookInfo hook.Info) (Operation, error)

	// NewSkipHook creates an operation to clear the unit's resolved flag, and
	// mark the supplied hook as completed successfully.
	NewSkipHook(hookInfo hook.Info) (Operation, error)

	// NewAction creates an operation to execute the supplied action.
	NewAction(actionId string) (Operation, error)

	// NewCommands creates an operation to execute the supplied script in the
	// indicated relation context, and pass the results back over the supplied
	// func.
	NewCommands(args CommandArgs, sendResponse CommandResponseFunc) (Operation, error)

	// NewUpdateRelations creates an operation to ensure the supplied relation
	// ids are known and tracked.
	NewUpdateRelations(ids []int) (Operation, error)

	// NewUpdateStorage creates an operation to ensure the supplied storage
	// tags are known and tracked.
	NewUpdateStorage(tags []names.StorageTag) (Operation, error)
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

// Callbacks exposes all the uniter code that's required by the various operations.
// It's far from cohesive, and fundamentally represents inappropriate coupling, so
// it's a prime candidate for future refactoring.
type Callbacks interface {
	ExecutionLocker

	// PrepareHook and CommitHook exist so that we can defer worrying about how
	// to untangle Uniter.relationers from everything else. They're only used by
	// RunHook operations.
	PrepareHook(info hook.Info) (name string, err error)
	CommitHook(info hook.Info) error

	// UpdateRelations exists so that we can encapsulate it in an operation.
	UpdateRelations(ids []int) error

	// NotifyHook* exist so that we can defer worrying about how to untangle the
	// callbacks inserted for uniter_test. They're only used by RunHook operations.
	NotifyHookCompleted(string, runner.Context)
	NotifyHookFailed(string, runner.Context)

	// InitializeMetricsCollector ensures that the collect-metrics hook timer is
	// up to date given the current deployed charm. It's only used in deploy
	// operations.
	InitializeMetricsCollector() error

	// The following methods exist primarily to allow us to test operation code
	// without using a live api connection.

	// FailAction marks the supplied action failed. It's only used by
	// RunActions operations.
	FailAction(actionId, message string) error

	// GetArchiveInfo is used to find out how to download a charm archive. It's
	// only used by Deploy operations.
	GetArchiveInfo(charmURL *corecharm.URL) (charm.BundleInfo, error)

	// SetCurrentCharm records intent to deploy a given charm. It must be called
	// *before* recording local state referencing that charm, to ensure there's
	// no path by which the state server can legitimately garbage collect that
	// charm or the service's settings for it. It's only used by Deploy operations.
	SetCurrentCharm(charmURL *corecharm.URL) error

	// ClearResolvedFlag notifies the state server that the uniter has accepted
	// the resolved attempt and is trying to progress. It's only used by Resolved
	// operations (which we generally expect to wrap other operations).
	ClearResolvedFlag() error
}

// StorageUpdater is an interface used for updating local knowledge of storage
// attachments.
type StorageUpdater interface {
	// UpdateStorage updates local knowledge of the storage attachments
	// with the specified tags.
	UpdateStorage([]names.StorageTag) error
}

// ExecutionLocker is an interface that provides a means of acquiring and
// releasing a machine-level lock. When acquiring the lock, the caller provides
// a message which will be recorded to aid in debugging.
type ExecutionLocker interface {
	// AcquireExecutionLock acquires the machine-level execution lock, and
	// returns a func that must be called to unlock it. It's used by all the
	// operations that execute external code.
	AcquireExecutionLock(message string) (unlock func(), err error)
}
