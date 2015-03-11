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
func (c *Client) Show(tags []names.StorageTag) ([]params.StorageInfo, error) {
	found := params.StorageShowResults{}
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i] = params.Entity{Tag: tag.String()}
	}
	if err := c.facade.FacadeCall("Show", params.Entities{Entities: entities}, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return c.convert(found.Results)
}
func (c *Client) convert(found []params.StorageShowResult) ([]params.StorageInfo, error) {
	var storages []params.StorageInfo
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
	found := params.StorageShowResults{}
	if err := c.facade.FacadeCall("List", nil, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return c.convert(found.Results)
}
