// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

// creator exists primarily to make the implementation of the Mode funcs more
// readable -- the general pattern is to switch to get a creator func (which
// doesn't allow for the possibility of error) and then to pass the chosen
// creator down to runOperation (which can then consistently create and run
// all the operations in the same way).
type creator func(factory operation.Factory) (operation.Operation, error)

// The following creator functions are all just dumb plumbing to support the
// Mode funcs.

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

func newCommandsOp(args operation.CommandArgs, sendResponse operation.CommandResponseFunc) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewCommands(args, sendResponse)
	}
}

func newActionOp(actionId string) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewAction(actionId)
	}
}

func newUpdateRelationsOp(ids []int) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewUpdateRelations(ids)
	}
}

func newUpdateStorageOp(tags []names.StorageTag) creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewUpdateStorage(tags)
	}
}

func newAcceptLeadershipOp() creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewAcceptLeadership()
	}
}

func newResignLeadershipOp() creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewResignLeadership()
	}
}

func newSendMetricsOp() creator {
	return func(factory operation.Factory) (operation.Operation, error) {
		return factory.NewSendMetrics()
	}
}
