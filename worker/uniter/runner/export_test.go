// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/juju/worker/uniter/runner/context"
)

func RunnerPaths(rnr Runner) context.Paths {
	return rnr.(*runner).paths
}
