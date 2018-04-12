// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
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

// NewMetricsAdderAPI creates a new API endpoint for adding metrics to state.
func NewMetricsAdderAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*MetricsAdderAPI, error) {
	// TODO(cmars): remove unit agent auth, once worker/metrics/sender manifold
	// can be righteously relocated to machine agent.
	if !authorizer.AuthMachineAgent() && !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &MetricsAdderAPI{
		state: st,
	}, nil
}

// AddMetricBatches implements the MetricsAdder interface.
func (api *MetricsAdderAPI) AddMetricBatches(args params.MetricBatchParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Batches)),
	}
	for i, batch := range args.Batches {
		tag, err := names.ParseUnitTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		metrics := make([]state.Metric, len(batch.Batch.Metrics))
		for j, metric := range batch.Batch.Metrics {
			metrics[j] = state.Metric{
				Key:    metric.Key,
				Value:  metric.Value,
				Time:   metric.Time,
				Labels: metric.Labels,
			}
		}
		_, err = api.state.AddMetrics(
			state.BatchParam{
				UUID:     batch.Batch.UUID,
				Created:  batch.Batch.Created,
				CharmURL: batch.Batch.CharmURL,
				Metrics:  metrics,
				Unit:     tag,
			},
		)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
