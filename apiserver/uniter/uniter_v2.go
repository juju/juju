// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface used by the uniter
// worker. This file contains the API facade version 2.

package uniter

import (
	"github.com/juju/errors"
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

// UniterAPIV2 implements the API version 2, used by the uniter worker.
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
		_, err = u.UniterAPIV1.st.AddMetrics(state.BatchParam{
			UUID:     batch.Batch.UUID,
			Created:  batch.Batch.Created,
			CharmURL: batch.Batch.CharmURL,
			Metrics:  metrics,
			Unit:     tag,
		})
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// NetworkConfig returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *UniterAPIV2) NetworkConfig(args params.RelationUnits) (params.UnitNetworkConfigResults, error) {
	result := params.UnitNetworkConfigResults{
		Results: make([]params.UnitNetworkConfigResult, len(args.RelationUnits)),
	}

	canAccess, err := u.UniterAPIV1.accessUnit()
	if err != nil {
		return params.UnitNetworkConfigResults{}, err
	}

	for i, rel := range args.RelationUnits {
		netConfig, err := u.getOneNetworkConfig(canAccess, rel.Relation, rel.Unit)
		if err == nil {
			result.Results[i].Config = netConfig
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPIV2) getOneNetworkConfig(canAccess common.AuthFunc, tagRel, tagUnit string) ([]params.NetworkConfig, error) {
	unitTag, err := names.ParseUnitTag(tagUnit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if !canAccess(unitTag) {
		return nil, common.ErrPerm
	}

	relTag, err := names.ParseRelationTag(tagRel)
	if err != nil {
		return nil, errors.Trace(err)
	}

	rel, err := u.UniterAPIV1.st.KeyRelation(relTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("got relation %q (%s) with endpoints: %+v", rel.Id(), rel.Tag(), rel.Endpoints())

	unit, err := u.getUnit(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineID, err := unit.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return u.oneMachineNetworkConfig(machineID)
}

func (u *UniterAPIV2) oneMachineNetworkConfig(id string) ([]params.NetworkConfig, error) {
	machine, err := u.UniterAPIV1.st.Machine(id)
	if err != nil {
		return nil, err
	}
	ifaces, err := machine.NetworkInterfaces()
	if err != nil {
		return nil, err
	}
	configs := make([]params.NetworkConfig, len(ifaces))
	for i, iface := range ifaces {
		configs[i] = params.NetworkConfig{
			MACAddress:    iface.MACAddress(),
			NetworkName:   iface.NetworkName(),
			InterfaceName: iface.RawInterfaceName(),
			Disabled:      iface.IsDisabled(),
			// TODO(dimitern) Add the rest of the fields, once we
			// store them in state.
		}
	}
	return configs, nil
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
