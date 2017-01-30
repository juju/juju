// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsdebug contains the implementation of an api endpoint
// for metrics debug functionality.
package metricsdebug

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MetricsDebug", 2, NewMetricsDebugAPI)
}

type metricsDebug interface {
	// MetricSummariesForUnit returns metric summaries for the given unit.
	MetricSummariesForUnit(unit string) ([]state.MetricSummary, error)

	// MetricSummariesForApplication returns metric summaries for the given application.
	MetricSummariesForApplication(application string) ([]state.MetricSummary, error)

	//MetricSummariesForModel returns all metrics summaries in the model.
	MetricSummariesForModel() ([]state.MetricSummary, error)

	// Unit returns the unit based on its name.
	Unit(string) (*state.Unit, error)

	// Application returns the application based on its name.
	Application(string) (*state.Application, error)
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
	resources facade.Resources,
	authorizer facade.Authorizer,
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
		summaries, err := api.state.MetricSummariesForModel()
		if err != nil {
			return results, errors.Annotate(err, "failed to get metrics")
		}
		return params.MetricResults{
			Results: []params.EntityMetrics{{
				Metrics: metricSummaryToMetricResult(summaries),
			}},
		}, nil
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		var summaries []state.MetricSummary
		switch tag.Kind() {
		case names.UnitTagKind:
			summaries, err = api.state.MetricSummariesForUnit(tag.Id())
			if err != nil {
				err = errors.Annotate(err, "failed to get metrics")
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		case names.ApplicationTagKind:
			summaries, err = api.state.MetricSummariesForApplication(tag.Id())
			if err != nil {
				err = errors.Annotate(err, "failed to get metrics")
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		default:
			err := errors.Errorf("invalid tag %v", arg.Tag)
			results.Results[i].Error = common.ServerError(err)
		}
		results.Results[i].Metrics = metricSummaryToMetricResult(summaries)
	}
	return results, nil
}

type byUnit []params.MetricResult

func (t byUnit) Len() int      { return len(t) }
func (t byUnit) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t byUnit) Less(i, j int) bool {
	return t[i].Unit < t[j].Unit
}

func metricSummaryToMetricResult(summaries []state.MetricSummary) []params.MetricResult {
	results := make([]params.MetricResult, len(summaries))
	for i, s := range summaries {
		results[i] = params.MetricResult{
			Key:   s.Key(),
			Value: s.Value(),
			Time:  s.Time(),
			Unit:  s.Unit(),
		}
	}
	return results

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
	case names.ApplicationTag:
		application, err := api.state.Application(entity.Id())
		if err != nil {
			return errors.Trace(err)
		}
		chURL, _ := application.CharmURL()
		if chURL.Schema != "local" {
			return errors.New("not a local charm")
		}
		units, err := application.AllUnits()
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
		return errors.Errorf("expected application or unit tag, got %T", entity)
	}
	return nil
}
