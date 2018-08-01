// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// AccountDetails retrieves details of the specified account.
// example of default name account that oracle provider has: default, cloud_storage.
func (c *Client) AccountDetails(name string) (resp response.Account, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty account name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["account"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllAccounts retrives details of the accounts that are in the
// specified identity domain. You can use this HTTP request to
// get details of the account that you must specify while creating a machine image.
func (c *Client) AllAccounts(filter []Filter) (resp response.AllAccounts, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/", c.endpoints["account"], c.identify)

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

// AllAccountNames retrieves names of all the accounts in the specified container.
func (c *Client) AllAccountNames() (resp response.DirectoryNames, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/", c.endpoints["account"], c.identify)

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

// DirectoryAccount retrieves the names of containers
// that contain objects that you can access. You can use this
// information to construct the multipart name of an object
func (c *Client) DirectoryAccount() (resp response.DirectoryNames, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := c.endpoints["account"] + "/"

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
