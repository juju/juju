// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/juju/worker/caasoperator/runner/context"
)

var (
	SearchHook = searchHook
)

func RunnerPaths(rnr Runner) context.Paths {
	return rnr.(*runner).paths
}
