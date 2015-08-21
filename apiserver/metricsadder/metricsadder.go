// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var (
	logger = loggo.GetLogger("juju.apiserver.metricsadder")
)

func init() {
	common.RegisterStandardFacade("MetricsAdder", 1, NewMetricsAdderAPI)
}

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
	resources *common.Resources,
	authorizer common.Authorizer,
) (*MetricsAdderAPI, error) {
	if !authorizer.AuthMachineAgent() {
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
				Key:   metric.Key,
				Value: metric.Value,
				Time:  metric.Time,
			}
		}
		unit, err := api.state.Unit(tag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		_, err = unit.AddMetrics(batch.Batch.UUID, batch.Batch.Created, batch.Batch.CharmURL, metrics)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
