// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/juju/v2/worker/uniter/runner/context"
)

var (
	MergeWindowsEnvironment = mergeWindowsEnvironment
	SearchHook              = discoverHookScript
	HookCommand             = hookCommand
	LookPath                = lookPath
)

func RunnerPaths(rnr Runner) context.Paths {
	return rnr.(*runner).paths
}
