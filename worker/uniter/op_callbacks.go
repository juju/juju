// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
)

// operationCallbacks implements operation.Callbacks, and exists entirely to
// keep those methods off the Uniter itself.
type operationCallbacks struct {
	u *Uniter
}

// AcquireExecutionLock is part of the operation.Callbacks interface.
func (opc *operationCallbacks) AcquireExecutionLock(message string) (func(), error) {
	// We want to make sure we don't block forever when locking, but take the
	// Uniter's tomb into account.
	checkTomb := func() error {
		select {
		case <-opc.u.tomb.Dying():
			return tomb.ErrDying
		default:
			// no-op to fall through to return.
		}
		return nil
	}
	message = fmt.Sprintf("%s: %s", opc.u.unit.Name(), message)
	if err := opc.u.hookLock.LockWithFunc(message, checkTomb); err != nil {
		return nil, err
	}
	return func() { opc.u.hookLock.Unlock() }, nil
}

// PrepareHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) PrepareHook(hi hook.Info) (string, error) {
	if hi.Kind.IsRelation() {
		return opc.u.relations.PrepareHook(hi)
	}
	if hi.Kind.IsStorage() {
		return opc.u.storage.PrepareHook(hi)
	}
	return string(hi.Kind), nil
}

// CommitHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) CommitHook(hi hook.Info) error {
	if hi.Kind.IsRelation() {
		return opc.u.relations.CommitHook(hi)
	}
	if hi.Kind.IsStorage() {
		return opc.u.storage.CommitHook(hi)
	}
	if hi.Kind == hooks.ConfigChanged {
		opc.u.ranConfigChanged = true
	}
	return nil
}

func notifyHook(hook string, ctx runner.Context, method func(string)) {
	if r, ok := ctx.HookRelation(); ok {
		remote, _ := ctx.RemoteUnitName()
		if remote != "" {
			remote = " " + remote
		}
		hook = hook + remote + " " + r.FakeId()
	}
	method(hook)
}

// NotifyHookCompleted is part of the operation.Callbacks interface.
func (opc *operationCallbacks) NotifyHookCompleted(hook string, ctx runner.Context) {
	if opc.u.observer != nil {
		notifyHook(hook, ctx, opc.u.observer.HookCompleted)
	}
}

// NotifyHookFailed is part of the operation.Callbacks interface.
func (opc *operationCallbacks) NotifyHookFailed(hook string, ctx runner.Context) {
	if opc.u.observer != nil {
		notifyHook(hook, ctx, opc.u.observer.HookFailed)
	}
}

// FailAction is part of the operation.Callbacks interface.
func (opc *operationCallbacks) FailAction(actionId, message string) error {
	if !names.IsValidAction(actionId) {
		return errors.Errorf("invalid action id %q", actionId)
	}
	tag := names.NewActionTag(actionId)
	err := opc.u.st.ActionFinish(tag, params.ActionFailed, nil, message)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		err = nil
	}
	return err
}

// GetArchiveInfo is part of the operation.Callbacks interface.
func (opc *operationCallbacks) GetArchiveInfo(charmURL *corecharm.URL) (charm.BundleInfo, error) {
	ch, err := opc.u.st.Charm(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ch, nil
}

// SetCurrentCharm is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetCurrentCharm(charmURL *corecharm.URL) error {
	return opc.u.f.SetCharm(charmURL)
}
