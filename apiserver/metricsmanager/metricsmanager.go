// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsmanager contains the implementation of an api endpoint
// for calling metrics functions in state.
package metricsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
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
	common.RegisterStandardFacade("MetricsManager", 1, newMetricsManagerAPI)
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
	clock         clock.Clock
}

var _ MetricsManager = (*MetricsManagerAPI)(nil)

// newMetricsManagerAPI wraps NewMetricsManagerAPI for RegisterStandardFacade.
func newMetricsManagerAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*MetricsManagerAPI, error) {
	return NewMetricsManagerAPI(st, resources, authorizer, clock.WallClock)
}

// NewMetricsManagerAPI creates a new API endpoint for calling metrics manager functions.
func NewMetricsManagerAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	clock clock.Clock,
) (*MetricsManagerAPI, error) {
	if !(authorizer.AuthMachineAgent() && authorizer.AuthModelManager()) {
		return nil, common.ErrPerm
	}

	// Allow access only to the current environment.
	accessEnviron := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag == nil {
				return false
			}
			return tag == st.ModelTag()
		}, nil
	}

	return &MetricsManagerAPI{
		state:         st,
		accessEnviron: accessEnviron,
		clock:         clock,
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
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		modelState := api.state
		if tag != api.state.ModelTag() {
			modelState, err = api.state.ForModel(tag)
			if err != nil {
				err = errors.Annotatef(err, "failed to access state for %s", tag)
				result.Results[i].Error = common.ServerError(err)
				continue
			}
		}

		err = modelState.CleanupOldMetrics()
		if err != nil {
			err = errors.Annotatef(err, "failed to cleanup old metrics for %s", tag)
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
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		modelState := api.state
		if tag != api.state.ModelTag() {
			modelState, err = api.state.ForModel(tag)
			if err != nil {
				err = errors.Annotatef(err, "failed to access state for %s", tag)
				result.Results[i].Error = common.ServerError(err)
				continue
			}
		}
		txVendorMetrics, err := transmitVendorMetrics(modelState)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = metricsender.SendMetrics(modelState, sender, api.clock, maxBatchesPerSend, txVendorMetrics)
		if err != nil {
			err = errors.Annotatef(err, "failed to send metrics for %s", tag)
			logger.Warningf("%v", err)
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

func transmitVendorMetrics(st *state.State) (bool, error) {
	cfg, err := st.ModelConfig()
	if err != nil {
		return false, errors.Annotatef(err, "failed to get model config for %s", st.ModelTag())
	}
	return cfg.TransmitVendorMetrics(), nil
}
