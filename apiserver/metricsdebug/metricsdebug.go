// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsdebug contains the implementation of an api endpoint
// for metrics debug functionality.
package metricsdebug

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var (
	logger = loggo.GetLogger("juju.apiserver.metricsdebug")
)

func init() {
	common.RegisterStandardFacade("MetricsDebug", 0, NewMetricsDebugAPI)
}

// MetricsDebug defines the methods on the metricsdebug API end point.
type MetricsDebug interface {
	// GetMetrics returns all metrics stored by the state server.
	GetMetrics(arg params.Entities) (params.MetricsResults, error)
}

// MetricsDebugAPI implements the metricsdebug interface and is the concrete
// implementation of the api end point.
type MetricsDebugAPI struct {
	state *state.State
}

var _ MetricsDebug = (*MetricsDebugAPI)(nil)

// NewMetricsDebugAPI creates a new API endpoint for calling metrics debug functions.
func NewMetricsDebugAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*MetricsDebugAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &MetricsDebugAPI{
		state: st,
	}, nil
}

type getMetricsEntity string

var (
	unitEntity    getMetricsEntity = "unit"
	serviceEntity getMetricsEntity = "service"
)

func (api *MetricsDebugAPI) unitOrService(entity string) (getMetricsEntity, error) {
	if _, err := api.state.Unit(entity); err == nil {
		return unitEntity, nil
	}
	if _, err := api.state.Service(entity); err == nil {
		return serviceEntity, nil
	}
	return "", errors.Errorf("entity %q not unit or service", entity)
}

// GetMetrics returns all metrics stored by the state server.
func (api *MetricsDebugAPI) GetMetrics(args params.Entities) (params.MetricsResults, error) {
	results := params.MetricsResults{
		Results: make([]params.MetricsResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return results, nil
	}
	for i, arg := range args.Entities {
		typ, err := api.unitOrService(arg.Tag)
		if err != nil {
			err = errors.Annotate(err, "failed to find unit")
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		var batches []state.MetricBatch
		if typ == unitEntity {
			batches, err = api.state.MetricBatchesForUnit(arg.Tag)
			if err != nil {
				err = errors.Annotate(err, "failed to get metrics")
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		} else if typ == serviceEntity {
			batches, err = api.state.MetricBatchesForService(arg.Tag)
			if err != nil {
				err = errors.Annotate(err, "failed to get metrics")
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		}
		results = params.MetricsResults{
			Results: make([]params.MetricsResult, len(batches)),
		}
		for i, mb := range batches {
			metricresult := make([]params.MetricResult, len(mb.Metrics()))
			for j, m := range mb.Metrics() {
				metricresult[j] = params.MetricResult{
					Key:   m.Key,
					Value: m.Value,
					Time:  m.Time,
				}
			}
			results.Results[i].Metrics = metricresult
		}
	}
	return results, nil
}
