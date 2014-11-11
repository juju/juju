// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager

import (
	"github.com/juju/juju/apiserver/metricsender"

	"github.com/juju/juju/state"
)

func PatchSender(s metricsender.MetricSender) {
	sender = s
}

func AddBuiltinMetrics(api *MetricsManagerAPI) ([]*state.MetricBatch, error) {
	return api.addBuiltinMetrics()
}
