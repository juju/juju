// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter/runner"
)

var (
	// NewCollect allows patching the function that creates the metric collection
	// entity.
	NewCollect = &newCollect

	// NewRecorder allows patching the function that creates the metric recorder.
	NewRecorder = &newRecorder
)

// NewHookContext returns a new hook context used to collect metrics.
func NewHookContext(unitName string, recorder spool.MetricRecorder) runner.Context {
	return &hookContext{unitName: unitName, recorder: recorder}
}
