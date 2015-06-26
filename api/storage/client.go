// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.storage")

// Client allows access to the storage API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the storage API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Storage")
	logger.Debugf("\nSTORAGE FRONT-END: %#v", frontend)
	logger.Debugf("\nSTORAGE BACK-END: %#v", backend)
	return &Client{ClientFacade: frontend, facade: backend}
}

// Show retrieves information about desired storage instances.
func (c *Client) Show(tags []names.StorageTag) ([]params.StorageDetails, error) {
	found := params.StorageDetailsResults{}
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i] = params.Entity{Tag: tag.String()}
	}
	if err := c.facade.FacadeCall("Show", params.Entities{Entities: entities}, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return c.convert(found.Results)
}

func (c *Client) convert(found []params.StorageDetailsResult) ([]params.StorageDetails, error) {
	var storages []params.StorageDetails
	var allErr params.ErrorResults
	for _, result := range found {
		if result.Error != nil {
			allErr.Results = append(allErr.Results, params.ErrorResult{result.Error})
			continue
		}
		storages = append(storages, result.Result)
	}
	return storages, allErr.Combine()
}

// List lists all storage.
func (c *Client) List() ([]params.StorageInfo, error) {
	found := params.StorageInfosResult{}
	if err := c.facade.FacadeCall("List", nil, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return found.Results, nil
}

// ListPools returns a list of pools that matches given filter.
// If no filter was provided, a list of all pools is returned.
func (c *Client) ListPools(providers, names []string) ([]params.StoragePool, error) {
	args := params.StoragePoolFilter{
		Names:     names,
		Providers: providers,
	}
	found := params.StoragePoolsResult{}
	if err := c.facade.FacadeCall("ListPools", args, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return found.Results, nil
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
func (c *Client) ListVolumes(machines []string) ([]params.VolumeItem, error) {
	tags := make([]string, len(machines))
	for i, one := range machines {
		tags[i] = names.NewMachineTag(one).String()
	}
	args := params.VolumeFilter{Machines: tags}
	found := params.VolumeItemsResult{}
	if err := c.facade.FacadeCall("ListVolumes", args, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return found.Results, nil
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
