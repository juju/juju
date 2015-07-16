// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

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

// PrepareHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) PrepareHook(hi hook.Info) (string, error) {
	name := string(hi.Kind)
	switch {
	case hi.Kind.IsRelation():
		var err error
		name, err = opc.u.relations.PrepareHook(hi)
		if err != nil {
			return "", err
		}
	case hi.Kind.IsStorage():
		if err := opc.u.storage.ValidateHook(hi); err != nil {
			return "", err
		}
		storageName, err := names.StorageName(hi.StorageId)
		if err != nil {
			return "", err
		}
		name = fmt.Sprintf("%s-%s", storageName, hi.Kind)
		// TODO(axw) if the agent is not installed yet,
		// set the status to "preparing storage".
	case hi.Kind == hooks.ConfigChanged:
		opc.u.f.DiscardConfigEvent()
	case hi.Kind == hook.LeaderSettingsChanged:
		opc.u.f.DiscardLeaderSettingsEvent()
	}
	return name, nil
}

// CommitHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) CommitHook(hi hook.Info) error {
	switch {
	case hi.Kind.IsRelation():
		return opc.u.relations.CommitHook(hi)
	case hi.Kind.IsStorage():
		return opc.u.storage.CommitHook(hi)
	case hi.Kind == hooks.ConfigChanged:
		opc.u.ranConfigChanged = true
	case hi.Kind == hook.LeaderSettingsChanged:
		opc.u.ranLeaderSettingsChanged = true
	}
	return nil
}

// UpdateRelations is part of the operation.Callbacks interface.
func (opc *operationCallbacks) UpdateRelations(ids []int) error {
	return opc.u.relations.Update(ids)
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

// ClearResolvedFlag is part of the operation.Callbacks interface.
func (opc *operationCallbacks) ClearResolvedFlag() error {
	return opc.u.f.ClearResolved()
}

// InitializeMetricsTimers is part of the operation.Callbacks interface.
func (opc *operationCallbacks) InitializeMetricsTimers() error {
	return opc.u.initializeMetricsTimers()
}

// SetExecutingStatus is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetExecutingStatus(message string) error {
	return setAgentStatus(opc.u, params.StatusExecuting, message, nil)
}
