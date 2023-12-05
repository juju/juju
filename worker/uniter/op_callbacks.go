// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner/context"
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
		err := opc.u.secretsTracker.PrepareHook(hi)
		if err != nil {
			return "", err
		}
	case hi.Kind == hooks.ConfigChanged:
		// TODO(axw)
		//opc.u.f.DiscardConfigEvent()
	case hi.Kind == hooks.LeaderSettingsChanged:
		// TODO(axw)
		//opc.u.f.DiscardLeaderSettingsEvent()
	}
	return name, nil
}

// CommitHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) CommitHook(hi hook.Info) error {
	switch {
	case hi.Kind == hooks.Start:
		opc.u.Probe.SetHasStarted(true)
	case hi.Kind == hooks.Stop:
		opc.u.Probe.SetHasStarted(false)
	case hi.Kind.IsWorkload():
	case hi.Kind.IsRelation():
		return opc.u.relationStateTracker.CommitHook(hi)
	case hi.Kind.IsStorage():
		return opc.u.storage.CommitHook(hi)
	case hi.Kind.IsSecret():
		return opc.u.secretsTracker.CommitHook(hi)
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
func (opc *operationCallbacks) FailAction(actionId, message string) error {
	if !names.IsValidAction(actionId) {
		return errors.Errorf("invalid action id %q", actionId)
	}
	tag := names.NewActionTag(actionId)
	err := opc.u.st.ActionFinish(tag, params.ActionFailed, nil, message)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) || params.IsCodeAlreadyExists(err) {
		err = nil
	}
	return err
}

func (opc *operationCallbacks) ActionStatus(actionId string) (string, error) {
	if !names.IsValidAction(actionId) {
		return "", errors.NotValidf("invalid action id %q", actionId)
	}
	tag := names.NewActionTag(actionId)
	return opc.u.st.ActionStatus(tag)
}

// GetArchiveInfo is part of the operation.Callbacks interface.
func (opc *operationCallbacks) GetArchiveInfo(url string) (charm.BundleInfo, error) {
	ch, err := opc.u.st.Charm(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ch, nil
}

// SetCurrentCharm is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetCurrentCharm(charmURL string) error {
	return opc.u.unit.SetCharmURL(charmURL)
}

// SetExecutingStatus is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetExecutingStatus(message string) error {
	return setAgentStatus(opc.u, status.Executing, message, nil)
}

// SetUpgradeSeriesStatus is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetUpgradeSeriesStatus(upgradeSeriesStatus model.UpgradeSeriesStatus, reason string) error {
	return setUpgradeSeriesStatus(opc.u, upgradeSeriesStatus, reason)
}

// RemoteInit is part of the operation.Callbacks interface.
func (opc *operationCallbacks) RemoteInit(runningStatus remotestate.ContainerRunningStatus, abort <-chan struct{}) error {
	if opc.u.modelType != model.CAAS || opc.u.sidecar {
		// Non CAAS model or sidecar CAAS model do not have remote init process.
		return nil
	}
	if opc.u.remoteInitFunc == nil {
		return nil
	}
	return opc.u.remoteInitFunc(runningStatus, abort)
}

// SetSecretRotated is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetSecretRotated(uri string, oldRevision int) error {
	return opc.u.secretsClient.SecretRotated(uri, oldRevision)
}

// SecretsRemoved is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SecretsRemoved(uris []string) error {
	return opc.u.secretsTracker.SecretsRemoved(uris)
}
