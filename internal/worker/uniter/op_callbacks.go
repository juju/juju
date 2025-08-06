// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

// operationCallbacks implements operation.Callbacks, and exists entirely to
// keep those methods off the Uniter itself.
type operationCallbacks struct {
	u *Uniter
}

// PrepareHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) PrepareHook(ctx stdcontext.Context, hi hook.Info) (string, error) {
	name := string(hi.Kind)
	switch {
	case hi.Kind.IsWorkload():
		name = fmt.Sprintf("%s-%s", hi.WorkloadName, hi.Kind)
	case hi.Kind.IsRelation():
		var err error
		name, err = opc.u.relationStateTracker.PrepareHook(hi)
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
	case hi.Kind.IsSecret():
		err := opc.u.secretsTracker.PrepareHook(ctx, hi)
		if err != nil {
			return "", err
		}
	case hi.Kind == hooks.ConfigChanged:
		// TODO(axw)
		//opc.u.f.DiscardConfigEvent()
	}
	return name, nil
}

// CommitHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) CommitHook(ctx stdcontext.Context, hi hook.Info) error {
	switch {
	case hi.Kind == hooks.Start:
		opc.u.Probe.SetHasStarted(true)
	case hi.Kind == hooks.Stop:
		opc.u.Probe.SetHasStarted(false)
	case hi.Kind.IsWorkload():
	case hi.Kind.IsRelation():
		return opc.u.relationStateTracker.CommitHook(ctx, hi)
	case hi.Kind.IsStorage():
		return opc.u.storage.CommitHook(ctx, hi)
	case hi.Kind.IsSecret():
		return opc.u.secretsTracker.CommitHook(ctx, hi)
	}
	return nil
}

func notifyHook(hook string, ctx context.Context, method func(string)) {
	if r, err := ctx.HookRelation(); err == nil {
		remote, _ := ctx.RemoteUnitName()
		if remote == "" {
			remote, _ = ctx.RemoteApplicationName()
		}
		if remote != "" {
			remote = " " + remote
		}
		hook = hook + remote + " " + r.FakeId()
	}
	method(hook)
}

// NotifyHookCompleted is part of the operation.Callbacks interface.
func (opc *operationCallbacks) NotifyHookCompleted(hook string, ctx context.Context) {
	if opc.u.observer != nil {
		notifyHook(hook, ctx, opc.u.observer.HookCompleted)
	}
}

// NotifyHookFailed is part of the operation.Callbacks interface.
func (opc *operationCallbacks) NotifyHookFailed(hook string, ctx context.Context) {
	if opc.u.observer != nil {
		notifyHook(hook, ctx, opc.u.observer.HookFailed)
	}
}

// FailAction is part of the operation.Callbacks interface.
func (opc *operationCallbacks) FailAction(ctx stdcontext.Context, actionId, message string) error {
	if !names.IsValidAction(actionId) {
		return errors.Errorf("invalid action id %q", actionId)
	}
	tag := names.NewActionTag(actionId)
	err := opc.u.client.ActionFinish(ctx, tag, params.ActionFailed, nil, message)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) || params.IsCodeAlreadyExists(err) {
		err = nil
	}
	return err
}

func (opc *operationCallbacks) ActionStatus(ctx stdcontext.Context, actionId string) (string, error) {
	if !names.IsValidAction(actionId) {
		return "", errors.NotValidf("invalid action id %q", actionId)
	}
	tag := names.NewActionTag(actionId)
	return opc.u.client.ActionStatus(ctx, tag)
}

// GetArchiveInfo is part of the operation.Callbacks interface.
func (opc *operationCallbacks) GetArchiveInfo(url string) (charm.BundleInfo, error) {
	ch, err := opc.u.client.Charm(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ch, nil
}

// SetCurrentCharm is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetCurrentCharm(ctx stdcontext.Context, charmURL string) error {
	return opc.u.unit.SetCharm(ctx, charmURL)
}

// SetExecutingStatus is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetExecutingStatus(ctx stdcontext.Context, message string) error {
	return setAgentStatus(ctx, opc.u, status.Executing, message, nil)
}

// SetSecretRotated is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetSecretRotated(ctx stdcontext.Context, uri string, oldRevision int) error {
	return opc.u.secretsClient.SecretRotated(ctx, uri, oldRevision)
}

// SecretsRemoved is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SecretsRemoved(ctx stdcontext.Context, uris []string) error {
	return opc.u.secretsTracker.SecretsRemoved(ctx, uris)
}
