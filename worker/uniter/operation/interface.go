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

type Operation interface {
	String() string
	Prepare(state State) (*State, error)
	Execute(state State) (*State, error)
	Commit(state State) (*State, error)
}

type Executor interface {
	State() State
	Run(Operation) error
	Skip(Operation) error
}

type Factory interface {
	NewDeploy(charmURL *corecharm.URL, kind Kind) (Operation, error)
	NewHook(hookInfo hook.Info) (Operation, error)
	NewAction(actionId string) (Operation, error)
	NewCommands(commands string, sendResponse CommandResponseFunc) (Operation, error)
}

type CommandResponseFunc func(*utilexec.ExecResponse, error)

// Callbacks...
type Callbacks interface {

	// AcquireExecutionLock...
	AcquireExecutionLock(message string) (unlock func(), err error)

	// GetRunner...
	GetRunner(ctx context.Context) context.Runner

	// PrepareHook and CommitHook exist so that I can defer worrying about how
	// to untangle Uniter.relationers from everything else.
	PrepareHook(info hook.Info) (name string, err error)
	CommitHook(info hook.Info) error

	// NotifyHook* exist so that I can defer worrying about how to untangle the
	// callbacks inserted for uniter_test.
	NotifyHookCompleted(string, context.Context)
	NotifyHookFailed(string, context.Context)

	// FailAction marks the supplied action failed. It exists so we can test
	// the operation package without involving the API.
	FailAction(actionId, message string) error

	// GetArchiveInfo is used to find out how to download a charm archive. It
	// exists so we can test the operation package without involving the API.
	GetArchiveInfo(charmURL *corecharm.URL) (charm.BundleInfo, error)

	// SetCurrentCharm...
	SetCurrentCharm(charmURL *corecharm.URL) error
}
