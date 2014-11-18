// Copyright 2012-2014 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

// runHookHelper implements operation.RunHookHelper, and exists entirely to
// move largely-irrelevant methods off the Uniter itself.
type runHookHelper struct {
	u *Uniter
}

func (rhh *runHookHelper) PrepareHook(hi hook.Info) (string, error) {
	if hi.Kind.IsRelation() {
		return rhh.u.relationers[hi.RelationId].PrepareHook(hi)
	}
	return string(hi.Kind), nil
}

func (rhh *runHookHelper) CommitHook(hi hook.Info) error {
	if hi.Kind.IsRelation() {
		if err := rhh.u.relationers[hi.RelationId].CommitHook(hi); err != nil {
			return err
		}
		if hi.Kind == hooks.RelationBroken {
			delete(rhh.u.relationers, hi.RelationId)
		}
	}
	if hi.Kind == hooks.ConfigChanged {
		rhh.u.ranConfigChanged = true
	}
	return nil
}

func (rhh *runHookHelper) NotifyHookCompleted(hook string, ctx context.Context) {
	if rhh.u.observer != nil {
		notifyHook(hook, ctx, rhh.u.observer.HookCompleted)
	}
}

func (rhh *runHookHelper) NotifyHookFailed(hook string, ctx context.Context) {
	if rhh.u.observer != nil {
		notifyHook(hook, ctx, rhh.u.observer.HookFailed)
	}
}

func notifyHook(hook string, ctx context.Context, method func(string)) {
	if r, ok := ctx.HookRelation(); ok {
		remote, _ := ctx.RemoteUnitName()
		if remote != "" {
			remote = " " + remote
		}
		hook = hook + remote + " " + r.FakeId()
	}
	method(hook)
}
