// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface used by the uniter
// worker. This file contains the API facade version 2.

package uniter

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.uniter")

func init() {
	common.RegisterStandardFacade("Uniter", 2, NewUniterAPIV2)
}

// UniterAPI implements the API version 2, used by the uniter worker.
type UniterAPIV2 struct {
	UniterAPIV1
	StorageAPI
}

// AddMetricBatches adds the metrics for the specified unit.
func (u *UniterAPIV2) AddMetricBatches(args params.MetricBatchParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Batches)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		logger.Warningf("failed to check unit access: %v", err)
		return params.ErrorResults{}, common.ErrPerm
	}
	for i, batch := range args.Batches {
		tag, err := names.ParseUnitTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		metrics := make([]state.Metric, len(batch.Batch.Metrics))
		for j, metric := range batch.Batch.Metrics {
			metrics[j] = state.Metric{
				Key:   metric.Key,
				Value: metric.Value,
				Time:  metric.Time,
			}
		}
		_, err = u.unit.AddMetrics(batch.Batch.UUID, batch.Batch.Created, batch.Batch.CharmURL, metrics)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// NewUniterAPIV2 creates a new instance of the Uniter API, version 2.
func NewUniterAPIV2(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPIV2, error) {
	baseAPI, err := NewUniterAPIV1(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	storageAPI, err := newStorageAPI(getStorageState(st), resources, baseAPI.accessUnit)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV2{
		UniterAPIV1: *baseAPI,
		StorageAPI:  *storageAPI,
	}, nil
}
