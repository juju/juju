// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacadeForFeature("Storage", 1, NewAPI, feature.Storage)
}

var getState = func(st *state.State) storageAccess {
	return stateShim{st}
}

// API implements the storage interface and is the concrete
// implementation of the api end point.
type API struct {
	storage    storageAccess
	authorizer common.Authorizer
}

// NewAPI returns a new storage API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		storage:    getState(st),
		authorizer: authorizer,
	}, nil
}

func (api *API) Show(entities params.Entities) (params.StorageShowResults, error) {
	all := []params.StorageShowResult{}

	for _, entity := range entities.Entities {
		instance, err := api.getStorageInstance(entity.Tag)
		if err != nil {
			all = append(all, params.StorageShowResult{Error: err})
			continue
		}
		attachments, err := api.getStorageAttachments(instance)
		if err != nil {
			all = append(all, params.StorageShowResult{Error: err})
			continue
		}
		if len(attachments) > 0 {
			// If any attachments were found for this storage instance,
			// return them instead.
			for _, attachment := range attachments {
				all = append(all, params.StorageShowResult{Result: attachment})
			}
			continue
		}
		// If we are here then this storage instance is unattached.
		all = append(all, params.StorageShowResult{Result: instance})
	}
	return params.StorageShowResults{Results: all}, nil
}

func (api *API) List() (params.StorageListResult, error) {
	nothing := params.StorageListResult{}

	stateInstances, err := api.storage.AllStorageInstances()
	if err != nil {
		return nothing, err
	}
	all := []params.StorageInfo{}
	for _, stateInstance := range stateInstances {
		instance := createParamsStorageInstance(stateInstance)
		attachments, err := api.getStorageAttachments(instance)
		if err != nil {
			return nothing, err
		}
		if len(attachments) > 0 {
			// If any attachments were found for this storage instance,
			// return them instead.
			all = append(all, attachments...)
			continue
		}
		// If we are here then this storage instance is unattached.
		all = append(all, instance)
	}

	return params.StorageListResult{Storages: all}, nil
}

func (api *API) getStorageAttachments(instance params.StorageInfo) ([]params.StorageInfo, *params.Error) {
	serverError := func(err error) *params.Error {
		return common.ServerError(errors.Annotatef(err, "getting attachments for owner %v", instance.OwnerTag))
	}
	aTag, err := names.ParseTag(instance.OwnerTag)
	if err != nil {
		return nil, serverError(common.ErrPerm)
	}

	unitTag, k := aTag.(names.UnitTag)
	if !k {
		// Definitely no attachments
		return nil, nil
	}

	stateAttachments, err := api.storage.StorageAttachments(unitTag)
	if err != nil {
		return nil, serverError(common.ErrPerm)
	}
	result := make([]params.StorageInfo, len(stateAttachments))
	for i, one := range stateAttachments {
		paramsStorageAttachment, err := createParamsStorageAttachment(instance, one)
		if err != nil {
			return nil, serverError(err)
		}
		result[i] = paramsStorageAttachment
	}
	return result, nil
}

func createParamsStorageAttachment(si params.StorageInfo, sa state.StorageAttachment) (params.StorageInfo, error) {
	result := params.StorageInfo{}
	result.StorageTag = sa.StorageInstance().String()
	if result.StorageTag != si.StorageTag {
		panic("attachment does not belong to storage instance")
	}
	result.UnitTag = sa.Unit().String()
	result.Attached = true
	info, err := sa.Info()
	// err here is not really an error:
	// it just means that this attachment is not provisioned
	if err == nil {
		result.Location = info.Location
		result.Provisioned = true
	}
	result.OwnerTag = si.OwnerTag
	result.Kind = si.Kind
	return result, nil
}

func (api *API) getStorageInstance(tag string) (params.StorageInfo, *params.Error) {
	nothing := params.StorageInfo{}
	serverError := func(err error) *params.Error {
		return common.ServerError(errors.Annotatef(err, "getting %v", tag))
	}
	aTag, err := names.ParseStorageTag(tag)
	if err != nil {
		return nothing, serverError(common.ErrPerm)
	}
	stateInstance, err := api.storage.StorageInstance(aTag)
	if err != nil {
		return nothing, serverError(common.ErrPerm)
	}
	return createParamsStorageInstance(stateInstance), nil
}

func createParamsStorageInstance(si state.StorageInstance) params.StorageInfo {
	result := params.StorageInfo{
		OwnerTag:   si.Owner().String(),
		StorageTag: si.Tag().String(),
		Kind:       params.StorageKind(si.Kind()),
	}
	return result
}
