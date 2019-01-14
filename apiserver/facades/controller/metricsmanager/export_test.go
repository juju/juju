// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager

import (
	"github.com/juju/juju/apiserver/facades/agent/metricsender"
)

func (api *MetricsManagerAPI) PatchSender(s metricsender.MetricSender) func() {
	prior := api.sender
	api.sender = s
	return func() {
		api.sender = prior
	}
}
