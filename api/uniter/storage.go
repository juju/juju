// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
)

type StorageAccessor struct {
	facade base.FacadeCaller
}

// NewStorageAccessor creates a StorageAccessor on the specified facade,
// and uses this name when calling through the caller.
func NewStorageAccessor(facade base.FacadeCaller) *StorageAccessor {
	return &StorageAccessor{facade}
}

func (sa *StorageAccessor) StorageInstances(unitTag names.Tag) ([]storage.StorageInstance, error) {
	if sa.facade.BestAPIVersion() < 2 {
		// StorageInstances() was introduced in UniterAPIV2.
		return nil, errors.NotImplementedf("StorageInstances() (need V2+)")
	}
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	}
	var results params.UnitStorageInstancesResults
	err := sa.facade.FacadeCall("UnitStorageInstances", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.UnitsStorageInstances) != 1 {
		panic(errors.Errorf("expected 1 result, got %d", len(results.UnitsStorageInstances)))
	}
	storageInstances := results.UnitsStorageInstances[0]
	if storageInstances.Error != nil {
		return nil, storageInstances.Error
	}
	return storageInstances.Instances, nil
}
