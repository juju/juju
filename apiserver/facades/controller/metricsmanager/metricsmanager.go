// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/metricsender"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var (
	logger            = loggo.GetLogger("juju.apiserver.metricsmanager")
	maxBatchesPerSend = metricsender.DefaultMaxBatchesPerSend()
	senderFactory     = metricsender.DefaultSenderFactory()
)

// MetricsManager defines the methods on the metricsmanager API end point.
type MetricsManager interface {
	CleanupOldMetrics(ctx context.Context, arg params.Entities) (params.ErrorResults, error)
	SendMetrics(ctx context.Context, args params.Entities) (params.ErrorResults, error)
}

// MetricsManagerAPI implements the metrics manager interface and is the concrete
// implementation of the api end point.
type MetricsManagerAPI struct {
	state       *state.State
	pool        *state.StatePool
	model       *state.Model
	accessModel common.GetAuthFunc
	clock       clock.Clock
	sender      metricsender.MetricSender
}

var _ MetricsManager = (*MetricsManagerAPI)(nil)

type modelBackend struct {
	*state.State
	*state.Model
}

// NewMetricsManagerAPI creates a new API endpoint for calling metrics manager functions.
func NewMetricsManagerAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	pool *state.StatePool,
	clock clock.Clock,
) (*MetricsManagerAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Allow access only to the current model.
	accessModel := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag == nil {
				return false
			}
			return tag == m.ModelTag()
		}, nil
	}

	config, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	sender := senderFactory(config.MeteringURL() + "/metrics")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MetricsManagerAPI{
		state:       st,
		pool:        pool,
		model:       m,
		accessModel: accessModel,
		clock:       clock,
		sender:      sender,
	}, nil
}

// CleanupOldMetrics removes old metrics from the collection.
// The single arg params is expected to contain and model uuid.
// Even though the call will delete all metrics across models
// it serves to validate that the connection has access to at least one model.
func (api *MetricsManagerAPI) CleanupOldMetrics(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := api.accessModel()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		modelState, release, err := api.getModelState(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		defer release()

		err = modelState.CleanupOldMetrics()
		if err != nil {
			err = errors.Annotatef(err, "failed to cleanup old metrics for %s", tag)
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// AddJujuMachineMetrics adds a metric that counts the number of
// non-container machines in the current model.
func (api *MetricsManagerAPI) AddJujuMachineMetrics(ctx context.Context) error {
	sla, err := api.state.SLACredential()
	if err != nil {
		return errors.Trace(err)
	}
	if len(sla) == 0 {
		return nil
	}
	allMachines, err := api.state.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}
	machineCount := 0
	osMachineCount := map[string]int{}
	for _, machine := range allMachines {
		ct := machine.ContainerType()
		if ct == instance.NONE || ct == "" {
			machineCount++
			osMachineCount[machine.Base().OS] = osMachineCount[machine.Base().OS] + 1
		}
	}
	t := clock.WallClock.Now()
	metrics := []state.Metric{{
		Key:   "juju-machines",
		Value: fmt.Sprintf("%d", machineCount),
		Time:  t,
	}}
	for osName, osMachineCount := range osMachineCount {
		metrics = append(metrics, state.Metric{
			Key:   "juju-" + osName + "-machines",
			Value: fmt.Sprintf("%d", osMachineCount),
			Time:  t,
		})
	}
	metricUUID, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = api.state.AddModelMetrics(state.ModelBatchParam{
		UUID:    metricUUID.String(),
		Created: t,
		Metrics: metrics,
	})
	return err
}

// SendMetrics will send any unsent metrics onto the metric collection service.
func (api *MetricsManagerAPI) SendMetrics(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	if err := api.AddJujuMachineMetrics(ctx); err != nil {
		logger.Warningf("failed to add juju-machines metrics: %v", err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := api.accessModel()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		modelState, release, err := api.getModelState(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		defer release()

		txVendorMetrics, err := transmitVendorMetrics(ctx, api.model)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		model, err := modelState.Model()
		if err != nil {
			return result, errors.Trace(err)
		}
		err = metricsender.SendMetrics(modelBackend{modelState, model}, api.sender, api.clock, maxBatchesPerSend, txVendorMetrics)
		if err != nil {
			err = errors.Annotatef(err, "failed to send metrics for %s", tag)
			logger.Warningf("%v", err)
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

func (api *MetricsManagerAPI) getModelState(tag names.Tag) (*state.State, func() bool, error) {
	if tag == api.model.ModelTag() {
		return api.state, func() bool { return false }, nil
	}
	st, err := api.pool.Get(tag.Id())
	if err != nil {
		return nil, nil, errors.Annotatef(err, "failed to access state for %s", tag)
	}
	return st.State, st.Release, nil
}

func transmitVendorMetrics(ctx context.Context, m *state.Model) (bool, error) {
	cfg, err := m.ModelConfig(ctx)
	if err != nil {
		return false, errors.Annotatef(err, "failed to get model config for %s", m.ModelTag())
	}
	return cfg.TransmitVendorMetrics(), nil
}
