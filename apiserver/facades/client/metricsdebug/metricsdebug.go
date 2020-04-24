// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsdebug contains the implementation of an api endpoint
// for metrics debug functionality.
package metricsdebug

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type metricsDebug interface {
	// MetricBatchesForUnit returns metric batches for the given unit.
	MetricBatchesForUnit(unit string) ([]state.MetricBatch, error)

	// MetricBatchesForApplication returns metric batches for the given application.
	MetricBatchesForApplication(application string) ([]state.MetricBatch, error)

	//MetricBatchesForModel returns all metrics batches in the model.
	MetricBatchesForModel() ([]state.MetricBatch, error)

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
		batches, err := api.state.MetricBatchesForModel()
		if err != nil {
			return results, errors.Annotate(err, "failed to get metrics")
		}
		return params.MetricResults{
			Results: []params.EntityMetrics{{
				Metrics: api.filterLastValuePerKeyPerUnit(batches),
			}},
		}, nil
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
		case names.ApplicationTagKind:
			batches, err = api.state.MetricBatchesForApplication(tag.Id())
			if err != nil {
				err = errors.Annotate(err, "failed to get metrics")
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		default:
			err := errors.Errorf("invalid tag %v", arg.Tag)
			results.Results[i].Error = common.ServerError(err)
		}
		results.Results[i].Metrics = api.filterLastValuePerKeyPerUnit(batches)
	}
	return results, nil
}

type byUnit []params.MetricResult

func (t byUnit) Len() int      { return len(t) }
func (t byUnit) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t byUnit) Less(i, j int) bool {
	if t[i].Unit == t[j].Unit {
		if t[i].Key == t[j].Key {
			return labelsKey(t[i].Labels) < labelsKey(t[j].Labels)
		}
		return t[i].Key < t[j].Key
	}
	return t[i].Unit < t[j].Unit
}

func labelsKey(m map[string]string) string {
	var result []string
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(result)
	return strings.Join(result, ",")
}

func (api *MetricsDebugAPI) filterLastValuePerKeyPerUnit(batches []state.MetricBatch) []params.MetricResult {
	metrics := []params.MetricResult{}
	for _, mb := range batches {
		for _, m := range mb.UniqueMetrics() {
			metrics = append(metrics, params.MetricResult{
				Key:    m.Key,
				Value:  m.Value,
				Time:   m.Time,
				Unit:   mb.Unit(),
				Labels: m.Labels,
			})
		}
	}
	uniq := map[string]params.MetricResult{}
	for _, m := range metrics {
		// we want unique keys per unit per metric per label combination
		uniq[fmt.Sprintf("%s-%s-%s", m.Key, m.Unit, labelsKey(m.Labels))] = m
	}
	results := make([]params.MetricResult, len(uniq))
	i := 0
	for _, m := range uniq {
		results[i] = m
		i++
	}
	sort.Sort(byUnit(results))
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
