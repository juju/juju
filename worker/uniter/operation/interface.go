// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/loggo"
	utilexec "github.com/juju/utils/exec"
	corecharm "gopkg.in/juju/charm.v4"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

var logger = loggo.GetLogger("juju.worker.uniter.operation")

// Operation encapsulates the stages of the various things the uniter can do,
// and the state changes that need to be recorded as they happen.
type Operation interface {
	String() string
	Prepare(state State) (*State, error)
	Execute(state State) (*State, error)
	Commit(state State) (*State, error)
}

// Executor records and exposes uniter state, and applies suitable changes as
// operations are run or skipped.
type Executor interface {
	State() State
	Run(Operation) error
	Skip(Operation) error
}

// Factory creates operations.
type Factory interface {
	NewDeploy(charmURL *corecharm.URL, kind Kind) (Operation, error)
	NewHook(hookInfo hook.Info) (Operation, error)
	NewAction(actionId string) (Operation, error)
	NewCommands(commands string, relationId int, remoteUnitName string, sendResponse CommandResponseFunc) (Operation, error)
}

// CommandResponseFunc is for marshalling command responses back to the source
// of the original request.
type CommandResponseFunc func(*utilexec.ExecResponse, error)

// Callbacks exposes all the uniter code that's required by the various operations.
// It's far from cohesive, and fundamentally represents inappropriate coupling, so
// it's a prime candidate for future refactoring.
type Callbacks interface {

	// AcquireExecutionLock acquires the machine-level execution lock, and
	// returns a func that must be called to unlock it. It's used by all the
	// operations that execute external code.
	AcquireExecutionLock(message string) (unlock func(), err error)

	// GetRunner returns a runner for the supplied context; it primarily exists
	// to allow operations to be tested without shelling out to run actual
	// external code. It's used by all the operations that execute external
	// code.
	GetRunner(ctx context.Context) context.Runner

	// PrepareHook and CommitHook exist so that we can defer worrying about how
	// to untangle Uniter.relationers from everything else. They're only used by
	// RunHook operations.
	PrepareHook(info hook.Info) (name string, err error)
	CommitHook(info hook.Info) error

	// NotifyHook* exist so that we can defer worrying about how to untangle the
	// callbacks inserted for uniter_test. They're only used by RunHook operations.
	NotifyHookCompleted(string, context.Context)
	NotifyHookFailed(string, context.Context)

	// FailAction marks the supplied action failed. It exists so we can test
	// the operation package without involving the API. It's only used by
	// RunActions operations.
	FailAction(actionId, message string) error

	// GetArchiveInfo is used to find out how to download a charm archive. It
	// exists so we can test the operation package without involving the API.
	// It's only used by Deploy operations.
	GetArchiveInfo(charmURL *corecharm.URL) (charm.BundleInfo, error)

	// SetCurrentCharm records intent to deploy a given charm. It must be called
	// *before* recording local state referencing that charm, to ensure there's
	// no path by which the state server can legitimately garbage collect that
	// charm or the service's settings for it. It's only used by Deploy operations.
	SetCurrentCharm(charmURL *corecharm.URL) error
}
