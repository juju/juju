// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsmanager contains the implementation of an api endpoint
// for calling metrics functions in state.
package metricsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var (
	logger            = loggo.GetLogger("juju.apiserver.metricsmanager")
	sender            = (state.MetricSender)(&metricsender.NopSender{})
	maxBatchesPerSend = 1000
)

func init() {
	common.RegisterStandardFacade("MetricsManager", 0, NewMetricsManagerAPI)
}

// MetricsManager defines the methods on the metricsmanager API end point.
type MetricsManager interface {
	CleanupOldMetrics(arg params.Entities) (params.ErrorResults, error)
	SendMetrics(args params.Entities) (params.ErrorResults, error)
}

// MetricsManagerAPI implements the metrics manager interface and is the concrete
// implementation of the api end point.
type MetricsManagerAPI struct {
	state *state.State
}

var _ MetricsManager = (*MetricsManagerAPI)(nil)

// NewMetricsManagerAPI creates a new API endpoint for calling metrics manager functions.
func NewMetricsManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*MetricsManagerAPI, error) {
	if !(authorizer.AuthMachineAgent() && authorizer.AuthEnvironManager()) {
		return nil, common.ErrPerm
	}

	return &MetricsManagerAPI{state: st}, nil
}

// CleanupOldMetrics removes old metrics from the collection.
// TODO (mattyw) Returns result with all the delete metrics
// The single arg params is expected to contain and environment uuid.
// Even though the call will delete all metrics across environments
// it serves to validate that the connection has access to at least one environment.
func (api *MetricsManagerAPI) CleanupOldMetrics(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	for i, arg := range args.Entities {
		if arg.Tag != api.state.EnvironTag().String() {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err := api.state.CleanupOldMetrics()
		if err != nil {
			err = errors.Annotate(err, "failed to cleanup old metrics")
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// SendMetrics will send any unsent metrics onto the metric collection service.
func (api *MetricsManagerAPI) SendMetrics(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	if len(args.Entities) == 0 {
		return result, nil
	}
	for i, arg := range args.Entities {
		if arg.Tag != api.state.EnvironTag().String() {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err := api.state.SendMetrics(sender, maxBatchesPerSend)
		if err != nil {
			err = errors.Annotate(err, "failed to send metrics")
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}
