// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names/v4"
)

// A UniterExecutionObserver gets the appropriate methods called when a hook
// is executed and either succeeds or fails.  Missing hooks don't get reported
// in this way.
type UniterExecutionObserver interface {
	HookCompleted(hookName string)
	HookFailed(hookName string)
}

// RebootQuerier is implemented by types that can deliver one-off machine
// reboot notifications to entities.
type RebootQuerier interface {
	Query(tag names.Tag) (bool, error)
}
