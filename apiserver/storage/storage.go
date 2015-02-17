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
	all := make([]params.StorageShowResult, len(entities.Entities))
	for i, entity := range entities.Entities {
		one := params.StorageShowResult{}
		instance, err := api.getStorageInstance(entity.Tag)
		if err != nil {
			one.Error = err
		}
		attachments, err := api.getStorageAttachments(instance)
		if err != nil {
			one.Error = err
		}

		if len(attachments) > 0 {
			one.Attachments = attachments
		} else {
			one.Instance = instance
		}
		all[i] = one
	}

	return params.StorageShowResults{Results: all}, nil
}

func (api *API) List() (params.StorageListResult, error) {
	nothing := params.StorageListResult{}

	stateInstances, err := api.storage.AllStorageInstances()
	if err != nil {
		return nothing, err
	}
	paramsAttachments := []params.StorageAttachment{}
	paramsInstances := []params.StorageInstance{}

	for _, stateInstance := range stateInstances {
		instance := createParamsStorageInstance(stateInstance)
		attachments, err := api.getStorageAttachments(instance)
		if err != nil {
			return nothing, err
		}
		if attachments != nil {
			paramsAttachments = append(paramsAttachments, attachments...)
		} else {
			paramsInstances = append(paramsInstances, instance)
		}
	}

	return params.StorageListResult{
		Instances:   paramsInstances,
		Attachments: paramsAttachments,
	}, nil
}

func (api *API) getStorageAttachments(instance params.StorageInstance) ([]params.StorageAttachment, *params.Error) {
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
	result := make([]params.StorageAttachment, len(stateAttachments))
	for i, one := range stateAttachments {
		paramsStorageAttachment, err := createParamsStorageAttachment(instance, one)
		if err != nil {
			return nil, serverError(err)
		}
		result[i] = paramsStorageAttachment
	}
	return result, nil
}

func createParamsStorageAttachment(si params.StorageInstance, sa state.StorageAttachment) (params.StorageAttachment, error) {
	result := params.StorageAttachment{}
	result.StorageTag = sa.StorageInstance().String()
	if result.StorageTag != si.StorageTag {
		panic("attachment does not belong to storage instance")
	}
	result.UnitTag = sa.Unit().String()
	info, err := sa.Info()
	// err here is not really an error:
	// it just means that this attachment is not provisioned
	if err != nil {
		result.Location = info.Location
	}
	result.OwnerTag = si.OwnerTag
	result.Kind = si.Kind
	return result, nil
}

func (api *API) getStorageInstance(tag string) (params.StorageInstance, *params.Error) {
	nothing := params.StorageInstance{}
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

func createParamsStorageInstance(si state.StorageInstance) params.StorageInstance {
	result := params.StorageInstance{
		OwnerTag:   si.Owner().String(),
		StorageTag: si.Tag().String(),
		Kind:       params.StorageKind(si.Kind()),
	}
	return result
}
