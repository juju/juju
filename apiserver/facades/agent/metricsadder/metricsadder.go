// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder

import (
	"context"

	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// MetricsAdder defines methods that are used to store metric batches in the state.
type MetricsAdder interface {
	// AddMetricBatches stores the specified metric batches in the state.
	AddMetricBatches(ctx context.Context, batches params.MetricBatchParams) (params.ErrorResults, error)
}

// MetricsAdderAPI implements the metrics adder interface and is the concrete
// implementation of the API end point.
type MetricsAdderAPI struct {
	state *state.State
}

var _ MetricsAdder = (*MetricsAdderAPI)(nil)

// AddMetricBatches implements the MetricsAdder interface.
func (api *MetricsAdderAPI) AddMetricBatches(ctx context.Context, args params.MetricBatchParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Batches)),
	}
	for i, batch := range args.Batches {
		tag, err := names.ParseUnitTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
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
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
