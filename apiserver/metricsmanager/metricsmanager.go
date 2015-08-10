// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsmanager contains the implementation of an api endpoint
// for calling metrics functions in state.
package metricsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var (
	logger            = loggo.GetLogger("juju.apiserver.metricsmanager")
	maxBatchesPerSend = metricsender.DefaultMaxBatchesPerSend()

	sender = metricsender.DefaultMetricSender()
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

	accessEnviron common.GetAuthFunc
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

	// Allow access only to the current environment.
	accessEnviron := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag == nil {
				return false
			}
			return tag == st.EnvironTag()
		}, nil
	}

	return &MetricsManagerAPI{
		state:         st,
		accessEnviron: accessEnviron,
	}, nil
}

// CleanupOldMetrics removes old metrics from the collection.
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
	canAccess, err := api.accessEnviron()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseEnvironTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = api.state.CleanupOldMetrics()
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
	canAccess, err := api.accessEnviron()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseEnvironTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = metricsender.SendMetrics(api.state, sender, maxBatchesPerSend)
		if err != nil {
			err = errors.Annotate(err, "failed to send metrics")
			logger.Warningf("%v", err)
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}
