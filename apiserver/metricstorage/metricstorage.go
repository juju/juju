// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsstorage contains the implementation of an API endpoint
// for storing metrics collected by the uniter in state.
package metricstorage

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var (
	logger = loggo.GetLogger("juju.apiserver.metricstorage")
)

func init() {
	common.RegisterStandardFacade("MetricStorage", 1, NewMetricStorageAPI)
}

// MetricStorageAPI is the implementation of the API endpoint.
type MetricStorageAPI struct {
	unit       *state.Unit
	accessUnit common.GetAuthFunc
}

// NewMetricStorageAPI creates a new API endpoint.
func NewMetricStorageAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*MetricStorageAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}

	tag := authorizer.GetAuthTag()
	if _, ok := tag.(names.UnitTag); ok {
		unit, err := st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		accessUnit := func() (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
		}
		return &MetricStorageAPI{
			unit:       unit,
			accessUnit: accessUnit,
		}, nil
	}
	return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
}

// AddMetricBatches stores the provided metric batches.
func (api *MetricStorageAPI) AddMetricBatches(args params.MetricBatchParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Batches)),
	}
	canAccess, err := api.accessUnit()
	if err != nil {
		return params.ErrorResults{}, common.ErrPerm
	}
	for i, batch := range args.Batches {
		tag, err := names.ParseUnitTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			metrics := make([]state.Metric, len(batch.Batch.Metrics))
			for j, metric := range batch.Batch.Metrics {
				metrics[j] = state.Metric{
					Key:   metric.Key,
					Value: metric.Value,
					Time:  metric.Time,
				}
			}
			_, err = api.unit.AddMetrics(batch.Batch.UUID, batch.Batch.Created, batch.Batch.CharmURL, metrics)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
