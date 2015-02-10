// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
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

// StorageAttachments returns the storage attachments for a collection of units.
func (s *StorageAPI) StorageAttachments(args params.Entities) (params.StorageAttachmentsResults, error) {
	canAccess, err := s.accessUnit()
	if err != nil {
		return params.StorageAttachmentsResults{}, err
	}
	result := params.StorageAttachmentsResults{
		Results: make([]params.StorageAttachmentsResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		storageAttachments, err := s.getOneUnitStorageAttachments(canAccess, entity.Tag)
		if err == nil {
			result.Results[i].Result = storageAttachments
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (s *StorageAPI) getOneUnitStorageAttachments(canAccess common.AuthFunc, unitTag string) ([]params.StorageAttachment, error) {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil || !canAccess(tag) {
		return nil, common.ErrPerm
	}
	stateStorageAttachments, err := s.st.StorageAttachments(tag)
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
	} else if err != nil {
		return nil, err
	}
	var result []params.StorageAttachment
	for _, stateStorageAttachment := range stateStorageAttachments {
		info, err := stateStorageAttachment.Info()
		if errors.IsNotProvisioned(err) {
			// don't return unprovisioned storage attachments
			continue
		} else if err != nil {
			return nil, err
		}
		stateStorageInstance, err := s.st.StorageInstance(stateStorageAttachment.StorageInstance())
		if err != nil {
			return nil, err
		}
		result = append(result, params.StorageAttachment{
			stateStorageAttachment.StorageInstance().String(),
			stateStorageInstance.Owner().String(),
			unitTag,
			params.StorageKind(stateStorageInstance.Kind()),
			info.Location,
		})
	}
	return result, nil
}
