// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"sort"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/utils"
)

// SecretTriggerTimeAssert is an assert function used as parameter for the
// generic WatcherC. It asserts that the `NextTriggerTime` in the
// `SecretTriggerChange` is "almost" as the passed expected value.
// It returns true if the assertion is correct (meaning that the listening
// loop in WatcherC can break).
func SecretTriggerTimeAssert(expected ...watcher.SecretTriggerChange) func(c *gc.C, received [][]watcher.SecretTriggerChange) bool {
	return func(c *gc.C, received [][]watcher.SecretTriggerChange) bool {
		flattened := utils.Flatten(received)
		if len(flattened) >= len(expected) {
			mc := jc.NewMultiChecker()
			mc.AddExpr(`_[_].NextTriggerTime`, jc.Almost, jc.ExpectedValue)
			c.Assert(flattened, mc, expected)
			return true
		}
		return false
	}
}

// SecretBackendRotateTriggerTimeAssert is an assert function used as parameter
// for the generic WatcherC. It asserts that the `NextTriggerTime` in the
// `SecretBackendRotateChange` is "almost" as the passed expected value.
// It returns true if the assertion is correct (meaning that the listening
// loop in WatcherC can break).
func SecretBackendRotateTriggerTimeAssert(expected ...watcher.SecretBackendRotateChange) func(c *gc.C, received [][]watcher.SecretBackendRotateChange) bool {
	return func(c *gc.C, received [][]watcher.SecretBackendRotateChange) bool {
		flattened := utils.Flatten(received)
		sort.Slice(flattened, func(i, j int) bool {
			return flattened[i].Name < flattened[j].Name
		})
		if len(flattened) >= len(expected) {
			mc := jc.NewMultiChecker()
			mc.AddExpr(`_[_].NextTriggerTime`, jc.Almost, jc.ExpectedValue)
			c.Assert(flattened, mc, expected)
			return true
		}
		return false
	}
}
