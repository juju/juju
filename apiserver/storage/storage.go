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

// API implements the storage interface and is the concrete
// implementation of the api end point.
type API struct {
	storage    storageAccess
	authorizer common.Authorizer
}

// createAPI returns a new storage API facade.
func createAPI(
	st storageAccess,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		storage:    st,
		authorizer: authorizer,
	}, nil
}

// NewAPI returns a new storage API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	return createAPI(getState(st), resources, authorizer)
}

func (api *API) Show(entities params.Entities) (params.StorageShowResults, error) {
	var all []params.StorageShowResult
	for _, entity := range entities.Entities {
		found, instance, err := api.getStorageInstance(entity.Tag)
		if err != nil {
			all = append(all, params.StorageShowResult{Error: err})
			continue
		}
		if found {
			all = append(all, api.createStorageShowResult(instance)...)
		}
	}
	return params.StorageShowResults{Results: all}, nil
}

func (api *API) List() (params.StorageShowResults, error) {
	stateInstances, err := api.storage.AllStorageInstances()
	if err != nil {
		return params.StorageShowResults{}, common.ServerError(err)
	}
	var infos []params.StorageShowResult
	for _, stateInstance := range stateInstances {
		instance := createParamsStorageInstance(stateInstance)
		infos = append(infos, api.createStorageShowResult(instance)...)
	}
	return params.StorageShowResults{Results: infos}, nil
}

func (api *API) createStorageShowResult(instance params.StorageInfo) []params.StorageShowResult {
	attachments, err := api.getStorageAttachments(instance)
	if err != nil {
		return []params.StorageShowResult{params.StorageShowResult{Error: err}}
	}
	if len(attachments) > 0 {
		// If any attachments were found for this storage instance,
		// return them instead.
		result := make([]params.StorageShowResult, len(attachments))
		for i, attachment := range attachments {
			result[i] = params.StorageShowResult{Result: attachment}
		}
		return result
	}
	// If we are here then this storage instance is unattached.
	return []params.StorageShowResult{params.StorageShowResult{Result: instance}}

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
		paramsStorageAttachment, err := api.createParamsStorageAttachment(instance, one)
		if err != nil {
			return nil, serverError(err)
		}
		result[i] = paramsStorageAttachment
	}
	return result, nil
}

func (api *API) createParamsStorageAttachment(si params.StorageInfo, sa state.StorageAttachment) (params.StorageInfo, error) {
	result := params.StorageInfo{}
	result.StorageTag = sa.StorageInstance().String()
	if result.StorageTag != si.StorageTag {
		panic("attachment does not belong to storage instance")
	}
	result.UnitTag = sa.Unit().String()
	result.Attached = true
	result.OwnerTag = si.OwnerTag
	result.Kind = si.Kind

	// This is only for provisioned attachments
	machineTag, err := api.storage.UnitAssignedMachine(sa.Unit())
	if err != nil {
		return params.StorageInfo{}, errors.Annotate(err, "getting unit for storage attachment")
	}
	info, err := common.StorageAttachmentInfo(api.storage, sa, machineTag)
	if err != nil {
		if errors.IsNotProvisioned(err) {
			// Not provisioned attachment is not an error
			return result, nil
		}
		return params.StorageInfo{}, errors.Annotate(err, "getting storage attachment info")
	}
	result.Location = info.Location
	if result.Location != "" {
		result.Provisioned = true
	}
	return result, nil
}

func (api *API) getStorageInstance(tag string) (bool, params.StorageInfo, *params.Error) {
	nothing := params.StorageInfo{}
	serverError := func(err error) *params.Error {
		return common.ServerError(errors.Annotatef(err, "getting %v", tag))
	}
	aTag, err := names.ParseStorageTag(tag)
	if err != nil {
		return false, nothing, serverError(common.ErrPerm)
	}
	stateInstance, err := api.storage.StorageInstance(aTag)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nothing, nil
		}
		return false, nothing, serverError(common.ErrPerm)
	}
	return true, createParamsStorageInstance(stateInstance), nil
}

func createParamsStorageInstance(si state.StorageInstance) params.StorageInfo {
	result := params.StorageInfo{
		OwnerTag:   si.Owner().String(),
		StorageTag: si.Tag().String(),
		Kind:       params.StorageKind(si.Kind()),
	}
	return result
}
