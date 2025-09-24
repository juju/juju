// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder

import (
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// MetricsAdder defines methods that are used to store metric batches in the state.
type MetricsAdder interface {
	// AddMetricBatches stores the specified metric batches in the state.
	AddMetricBatches(batches params.MetricBatchParams) (params.ErrorResults, error)
}

// MetricsAdderAPI implements the metrics adder interface and is the concrete
// implementation of the API end point.
type MetricsAdderAPI struct {
	state *state.State
}

var _ MetricsAdder = (*MetricsAdderAPI)(nil)

// AddMetricBatches implements the MetricsAdder interface.
// This is a noop as of 3.6.10 because metric functionality is removed.
func (api *MetricsAdderAPI) AddMetricBatches(args params.MetricBatchParams) (params.ErrorResults, error) {
	return params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Batches)),
	}, nil
}
