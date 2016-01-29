// Copyright 2016 Canonical Ltd.
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
	common.RegisterStandardFacade("MetricsDebug", 1, NewMetricsDebugAPI)
}

type metricsDebug interface {
	// MetricBatchesForUnit returns metric batches for the given unit.
	MetricBatchesForUnit(unit string) ([]state.MetricBatch, error)

	// MetricBatchesForService returns metric batches for the given service.
	MetricBatchesForService(service string) ([]state.MetricBatch, error)

	// Unit returns the unit based on its name.
	Unit(string) (*state.Unit, error)

	// Service returns the service based on its name.
	Service(string) (*state.Service, error)
}

// MetricsDebug defines the methods on the metricsdebug API end point.
type MetricsDebug interface {
	// GetMetrics returns all metrics stored by the state server.
	GetMetrics(arg params.Entities) (params.MetricResults, error)

	// SetMeterStatus will set the meter status on the given entity tag.
	SetMeterStatus(params.MeterStatusParams) (params.ErrorResults, error)
}

// MetricsDebugAPI implements the metricsdebug interface and is the concrete
// implementation of the api end point.
type MetricsDebugAPI struct {
	state metricsDebug
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

// GetMetrics returns all metrics stored by the state server.
func (api *MetricsDebugAPI) GetMetrics(args params.Entities) (params.MetricResults, error) {
	results := params.MetricResults{
		Results: make([]params.EntityMetrics, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return results, nil
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		var batches []state.MetricBatch
		switch tag.Kind() {
		case names.UnitTagKind:
			batches, err = api.state.MetricBatchesForUnit(tag.Id())
			if err != nil {
				err = errors.Annotate(err, "failed to get metrics")
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		case names.ServiceTagKind:
			batches, err = api.state.MetricBatchesForService(tag.Id())
			if err != nil {
				err = errors.Annotate(err, "failed to get metrics")
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		default:
			err := errors.Errorf("invalid tag %v", arg.Tag)
			results.Results[i].Error = common.ServerError(err)
		}
		metricCount := 0
		for _, b := range batches {
			metricCount += len(b.Metrics())
		}
		metrics := make([]params.MetricResult, metricCount)
		ix := 0
		for _, mb := range batches {
			for _, m := range mb.Metrics() {
				metrics[ix] = params.MetricResult{
					Key:   m.Key,
					Value: m.Value,
					Time:  m.Time,
				}
				ix++
			}
			results.Results[i].Metrics = metrics
		}
	}
	return results, nil
}

// SetMeterStatus sets meter statuses for entities.
func (api *MetricsDebugAPI) SetMeterStatus(args params.MeterStatusParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Statuses)),
	}
	for i, arg := range args.Statuses {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		err = api.setEntityMeterStatus(tag, state.MeterStatus{
			Code: state.MeterStatusFromString(arg.Code),
			Info: arg.Info,
		})
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}

func (api *MetricsDebugAPI) setEntityMeterStatus(entity names.Tag, status state.MeterStatus) error {
	switch entity := entity.(type) {
	case names.UnitTag:
		unit, err := api.state.Unit(entity.Id())
		if err != nil {
			return errors.Trace(err)
		}
		chURL, found := unit.CharmURL()
		if !found {
			return errors.New("no charm url")
		}
		if chURL.Schema != "local" {
			return errors.New("not a local charm")
		}
		err = unit.SetMeterStatus(status.Code.String(), status.Info)
		if err != nil {
			return errors.Trace(err)
		}
	case names.ServiceTag:
		service, err := api.state.Service(entity.Id())
		if err != nil {
			return errors.Trace(err)
		}
		chURL, _ := service.CharmURL()
		if chURL.Schema != "local" {
			return errors.New("not a local charm")
		}
		units, err := service.AllUnits()
		if err != nil {
			return errors.Trace(err)
		}
		for _, unit := range units {
			err := unit.SetMeterStatus(status.Code.String(), status.Info)
			if err != nil {
				return errors.Trace(err)
			}
		}
	default:
		return errors.Errorf("expected service or unit tag, got %T", entity)
	}
	return nil
}
