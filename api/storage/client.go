// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
)

// Client allows access to the storage API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the storage API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Storage")
	return &Client{ClientFacade: frontend, facade: backend}
}

// StorageDetails retrieves details about desired storage instances.
func (c *Client) StorageDetails(tags []names.StorageTag) ([]params.StorageDetailsResult, error) {
	found := params.StorageDetailsResults{}
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i] = params.Entity{Tag: tag.String()}
	}
	if err := c.facade.FacadeCall("StorageDetails", params.Entities{Entities: entities}, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return found.Results, nil
}

// ListStorageDetails lists all storage.
func (c *Client) ListStorageDetails() ([]params.StorageDetails, error) {
	args := params.StorageFilters{
		[]params.StorageFilter{{}}, // one empty filter
	}
	var results params.StorageDetailsListResults
	if err := c.facade.FacadeCall("ListStorageDetails", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf(
			"expected 1 result, got %d",
			len(results.Results),
		)
	}
	if results.Results[0].Error != nil {
		return nil, errors.Trace(results.Results[0].Error)
	}
	return results.Results[0].Result, nil
}

// ListPools returns a list of pools that matches given filter.
// If no filter was provided, a list of all pools is returned.
func (c *Client) ListPools(providers, names []string) ([]params.StoragePool, error) {
	args := params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names:     names,
			Providers: providers,
		}},
	}
	var results params.StoragePoolsResults
	if err := c.facade.FacadeCall("ListPools", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return nil, err
	}
	return results.Results[0].Result, nil
}

// CreatePool creates pool with specified parameters.
func (c *Client) CreatePool(pname, provider string, attrs map[string]interface{}) error {
	args := params.StoragePool{
		Name:     pname,
		Provider: provider,
		Attrs:    attrs,
	}
	return c.facade.FacadeCall("CreatePool", args, nil)
}

// ListVolumes lists volumes for desired machines.
// If no machines provided, a list of all volumes is returned.
func (c *Client) ListVolumes(machines []string) ([]params.VolumeDetailsListResult, error) {
	filters := make([]params.VolumeFilter, len(machines))
	for i, machine := range machines {
		filters[i].Machines = []string{names.NewMachineTag(machine).String()}
	}
	if len(filters) == 0 {
		filters = []params.VolumeFilter{{}}
	}
	args := params.VolumeFilters{filters}
	var results params.VolumeDetailsListResults
	if err := c.facade.FacadeCall("ListVolumes", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(filters) {
		return nil, errors.Errorf(
			"expected %d result(s), got %d",
			len(filters), len(results.Results),
		)
	}
	return results.Results, nil
}

// ListFilesystems lists filesystems for desired machines.
// If no machines provided, a list of all filesystems is returned.
func (c *Client) ListFilesystems(machines []string) ([]params.FilesystemDetailsListResult, error) {
	filters := make([]params.FilesystemFilter, len(machines))
	for i, machine := range machines {
		filters[i].Machines = []string{names.NewMachineTag(machine).String()}
	}
	if len(filters) == 0 {
		filters = []params.FilesystemFilter{{}}
	}
	args := params.FilesystemFilters{filters}
	var results params.FilesystemDetailsListResults
	if err := c.facade.FacadeCall("ListFilesystems", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(filters) {
		return nil, errors.Errorf(
			"expected %d result(s), got %d",
			len(filters), len(results.Results),
		)
	}
	return results.Results, nil
}

// AddToUnit adds specified storage to desired units.
func (c *Client) AddToUnit(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
	out := params.ErrorResults{}
	in := params.StoragesAddParams{Storages: storages}
	err := c.facade.FacadeCall("AddToUnit", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}

// Attach attaches existing storage to a unit.
func (c *Client) Attach(unitId string, storageIds []string) ([]params.ErrorResult, error) {
	in := params.StorageAttachmentIds{
		make([]params.StorageAttachmentId, len(storageIds)),
	}
	if !names.IsValidUnit(unitId) {
		return nil, errors.NotValidf("unit ID %q", unitId)
	}
	for i, storageId := range storageIds {
		if !names.IsValidStorage(storageId) {
			return nil, errors.NotValidf("storage ID %q", storageId)
		}
		in.Ids[i] = params.StorageAttachmentId{
			StorageTag: names.NewStorageTag(storageId).String(),
			UnitTag:    names.NewUnitTag(unitId).String(),
		}
	}
	out := params.ErrorResults{}
	if err := c.facade.FacadeCall("Attach", in, &out); err != nil {
		return nil, errors.Trace(err)
	}
	if len(out.Results) != len(storageIds) {
		return nil, errors.Errorf(
			"expected %d result(s), got %d",
			len(storageIds), len(out.Results),
		)
	}
	return out.Results, nil
}

// Destroy destroys specified storage entities.
func (c *Client) Destroy(storageIds []string, destroyAttached bool) ([]params.ErrorResult, error) {
	for _, id := range storageIds {
		if !names.IsValidStorage(id) {
			return nil, errors.NotValidf("storage ID %q", id)
		}
	}
	results := params.ErrorResults{}
	var args interface{}
	if c.BestAPIVersion() <= 3 {
		// In version 3, destroyAttached is ignored; removing
		// storage always causes detachment.
		entities := make([]params.Entity, len(storageIds))
		for i, id := range storageIds {
			entities[i].Tag = names.NewStorageTag(id).String()
		}
		args = params.Entities{entities}
	} else {
		storage := make([]params.DestroyStorageInstance, len(storageIds))
		for i, id := range storageIds {
			storage[i] = params.DestroyStorageInstance{
				Tag:             names.NewStorageTag(id).String(),
				DestroyAttached: destroyAttached,
			}
		}
		args = params.DestroyStorage{storage}
	}
	if err := c.facade.FacadeCall("Destroy", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(storageIds) {
		return nil, errors.Errorf(
			"expected %d result(s), got %d",
			len(storageIds), len(results.Results),
		)
	}
	return results.Results, nil
}

// Detach detaches the specified storage entities.
func (c *Client) Detach(storageIds []string) ([]params.ErrorResult, error) {
	results := params.ErrorResults{}
	args := make([]params.StorageAttachmentId, len(storageIds))
	for i, id := range storageIds {
		if !names.IsValidStorage(id) {
			return nil, errors.NotValidf("storage ID %q", id)
		}
		args[i] = params.StorageAttachmentId{
			StorageTag: names.NewStorageTag(id).String(),
		}
	}
	if err := c.facade.FacadeCall(
		"Detach",
		params.StorageAttachmentIds{args},
		&results,
	); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(storageIds) {
		return nil, errors.Errorf(
			"expected %d result(s), got %d",
			len(storageIds), len(results.Results),
		)
	}
	return results.Results, nil
}

// Import imports storage into the model.
func (c *Client) ImportStorage(
	kind storage.StorageKind,
	storagePool string,
	storageProviderId string,
	storageName string,
) (names.StorageTag, error) {
	var results params.ImportStorageResults
	args := params.BulkImportStorageParams{
		[]params.ImportStorageParams{{
			StorageName: storageName,
			Kind:        params.StorageKind(kind),
			Pool:        storagePool,
			ProviderId:  storageProviderId,
		}},
	}
	if err := c.facade.FacadeCall("Import", args, &results); err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return names.StorageTag{}, errors.Errorf(
			"expected 1 result, got %d",
			len(results.Results),
		)
	}
	if err := results.Results[0].Error; err != nil {
		return names.StorageTag{}, err
	}
	return names.ParseStorageTag(results.Results[0].Result.StorageTag)
}
