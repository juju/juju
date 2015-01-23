// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// StorageAPI provides access to the Storage API facade.
type StorageAPI struct {
	st         storageStateInterface
	resources  *common.Resources
	accessUnit common.GetAuthFunc
}

// NewStorageAPI creates a new server-side Storage API facade.
func NewStorageAPI(
	st *state.State,
	resources *common.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {

	return &StorageAPI{
		st:         getStorageState(st),
		resources:  resources,
		accessUnit: accessUnit,
	}, nil
}

func (s *StorageAPI) UnitStorageInstances(args params.Entities) (params.UnitStorageInstancesResults, error) {
	canAccess, err := s.accessUnit()
	if err != nil {
		return params.UnitStorageInstancesResults{}, err
	}
	result := params.UnitStorageInstancesResults{
		UnitsStorageInstances: make([]params.UnitStorageInstances, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result.UnitsStorageInstances[i] = s.getOneUnitStorageInstances(canAccess, entity.Tag)
	}
	return result, nil
}

func (s *StorageAPI) getOneUnitStorageInstances(canAccess common.AuthFunc, unitTag string) params.UnitStorageInstances {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil {
		return params.UnitStorageInstances{Error: common.ServerError(common.ErrPerm)}
	}
	if !canAccess(tag) {
		return params.UnitStorageInstances{Error: common.ServerError(common.ErrPerm)}
	}
	unit, err := s.st.Unit(tag.Id())
	if err != nil {
		return params.UnitStorageInstances{Error: common.ServerError(common.ErrPerm)}
	}
	var result params.UnitStorageInstances
	for _, storageId := range unit.StorageInstanceIds() {
		stateStorageInstance, err := s.st.StorageInstance(storageId)
		if err != nil {
			result.Error = common.ServerError(err)
			result.Instances = nil
			break
		}
		storageInstance := storage.StorageInstance{
			stateStorageInstance.Id(),
			storage.StorageKind(stateStorageInstance.Kind()),
			"", //TODO - add Location
		}
		result.Instances = append(result.Instances, storageInstance)
	}
	return result
}
