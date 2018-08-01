// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// DeleteInstance shuts down an instance and removes it permanently
// from the system.
// Example of name f653a677-b566-4f92-8e93-71d47b364119
func (c *Client) DeleteInstance(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty instance name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["instance"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// AllInstances retrieves details of the instances that are in the specified
// container and match the specified query criteria.
// If you don't specify any query criteria, then details
// of all the instances in the container are displayed.
// You can filter by tags.
func (c *Client) AllInstances(filter []Filter) (resp response.AllInstances, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["instance"], c.identify, c.username)

	if err = c.request(paramsRequest{
		url:    url,
		verb:   "GET",
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// InstanceDetails retrieves details of the specified instance.
// Name is the form of dev-name/uuid
func (c *Client) InstanceDetails(name string) (resp response.Instance, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty instance name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["instance"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllInstanceNames retrieves the names of objects and subcontainers
// that you can access in the specified container.
func (c *Client) AllInstanceNames() (resp response.DirectoryNames, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/", c.endpoints["instance"], c.identify, c.username)

	if err = c.request(paramsRequest{
		directory: true,
		url:       url,
		verb:      "GET",
		resp:      &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
