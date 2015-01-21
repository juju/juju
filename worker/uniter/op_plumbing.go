// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

// These functions are all just dumb plumbing to support the Mode funcs. See
// the doc comments for creator and Uniter.runOperation for justification.

func newInstallOp(charmURL *charm.URL) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewInstall(charmURL)
	}
}

func newUpgradeOp(charmURL *charm.URL) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewUpgrade(charmURL)
	}
}

func newRevertUpgradeOp(charmURL *charm.URL) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewRevertUpgrade(charmURL)
	}
}

func newResolvedUpgradeOp(charmURL *charm.URL) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewResolvedUpgrade(charmURL)
	}
}

func newSimpleRunHookOp(kind hooks.Kind) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewRunHook(hook.Info{Kind: kind})
	}
}

func newRunHookOp(hookInfo hook.Info) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewRunHook(hookInfo)
	}
}

func newRetryHookOp(hookInfo hook.Info) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewRetryHook(hookInfo)
	}
}

func newSkipHookOp(hookInfo hook.Info) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewSkipHook(hookInfo)
	}
}

func newActionOp(actionId string) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewAction(actionId)
	}
}
