// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// StoragePropertyDetails retrieves details of the specified storage property
func (c *Client) StoragePropertyDetails(
	name string,
) (resp response.StorageProperty, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty storage property name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["storageproperty"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllStorageProperties retrieves details of the storage properties
// that are available in the specified container
func (c *Client) AllStorageProperties(
	filter []Filter,
) (resp response.AllStorageProperties, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/oracle/public/", c.endpoints["storageproperty"])

	if err = c.request(paramsRequest{
		verb:   "GET",
		url:    url,
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllStoragePropertyNames retrieves the names of objects and subcontainers
// that you can access in the specified container.
func (c *Client) AllStoragePropertyNames() (resp response.DirectoryNames, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := c.endpoints["storageproperty"] + "/oracle/"

	if err = c.request(paramsRequest{
		directory: true,
		verb:      "GET",
		url:       url,
		resp:      &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
