// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/juju/status"
	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/runner"
)

// operationCallbacks implements operation.Callbacks, and exists entirely to
// keep those methods off the operator itself.
type operationCallbacks struct {
	op *caasOperator
}

// PrepareHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) PrepareHook(hi hook.Info) (string, error) {
	name := string(hi.Kind)
	switch {
	case hi.Kind.IsRelation():
		// TODO(caas)
		//var err error
		//name, err = opc.op.relations.PrepareHook(hi)
		//if err != nil {
		//	return "", err
		//}
	}
	return name, nil
}

// CommitHook is part of the operation.Callbacks interface.
func (opc *operationCallbacks) CommitHook(hi hook.Info) error {
	// TODO(caas)
	//switch {
	//case hi.Kind.IsRelation():
	//	return opc.op.relations.CommitHook(hi)
	//}
	return nil
}

func notifyHook(hook string, ctx runner.Context, method func(string)) {
	if r, err := ctx.HookRelation(); err == nil {
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
	if opc.op.observer != nil {
		notifyHook(hook, ctx, opc.op.observer.HookCompleted)
	}
}

// NotifyHookFailed is part of the operation.Callbacks interface.
func (opc *operationCallbacks) NotifyHookFailed(hook string, ctx runner.Context) {
	if opc.op.observer != nil {
		notifyHook(hook, ctx, opc.op.observer.HookFailed)
	}
}

// SetExecutingStatus is part of the operation.Callbacks interface.
func (opc *operationCallbacks) SetExecutingStatus(message string) error {
	return setAgentStatus(opc.op, status.Executing, message, nil)
}
