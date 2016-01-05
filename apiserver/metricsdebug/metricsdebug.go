// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsdebug contains the implementation of an api endpoint
// for metrics debug functionality.
package metricsdebug

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

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

	accessEnviron common.GetAuthFunc
}

var _ MetricsDebug = (*MetricsDebugAPI)(nil)

// NewMetricsDebugAPI creates a new API endpoint for calling metrics debug functions.
func NewMetricsDebugAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*MetricsDebugAPI, error) {
	if !(authorizer.AuthMachineAgent() && authorizer.AuthEnvironManager()) {
		// TODO (mattyw) Needs to be different.
		return nil, common.ErrPerm
	}

	// Allow access only to the current environment.
	accessEnviron := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag == nil {
				return false
			}
			return tag == st.EnvironTag()
		}, nil
	}

	return &MetricsDebugAPI{
		state:         st,
		accessEnviron: accessEnviron,
	}, nil
}

// GetMetrics returns all metrics stored by the state server.
func (api *MetricsDebugAPI) GetMetrics(args params.Entities) (params.MetricsResults, error) {
	results := params.MetricsResults{
		Results: make([]params.MetricsResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return results, nil
	}
	canAccess, err := api.accessEnviron()
	if err != nil {
		return results, err
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseEnvironTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		batches, err := api.state.MetricBatches()
		if err != nil {
			err = errors.Annotate(err, "failed to get metrics")
			results.Results[i].Error = common.ServerError(err)
			continue
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
