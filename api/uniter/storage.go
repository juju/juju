// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

type StorageAccessor struct {
	facade base.FacadeCaller
}

// NewStorageAccessor creates a StorageAccessor on the specified facade,
// and uses this name when calling through the caller.
func NewStorageAccessor(facade base.FacadeCaller) *StorageAccessor {
	return &StorageAccessor{facade}
}

// StorageAttachments returns the storage instances attached to a unit.
func (sa *StorageAccessor) StorageAttachments(unitTag names.Tag) ([]params.StorageAttachment, error) {
	if sa.facade.BestAPIVersion() < 2 {
		// StorageAttachments() was introduced in UniterAPIV2.
		return nil, errors.NotImplementedf("StorageAttachments() (need V2+)")
	}
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	}
	var results params.StorageAttachmentsResults
	err := sa.facade.FacadeCall("StorageAttachments", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		panic(errors.Errorf("expected 1 result, got %d", len(results.Results)))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Result, nil
}
