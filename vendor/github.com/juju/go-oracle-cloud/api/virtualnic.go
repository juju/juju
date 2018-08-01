// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// VirtualNicDetails retrives a virtual nic with that has a given name
func (c *Client) VirtualNicDetails(name string) (resp response.VirtualNic, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty virtual nic name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["virtualnic"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllVirtualNics returns all virtual nic that are in the oracle account
func (c *Client) AllVirtualNics(filter []Filter) (resp response.AllVirtualNics, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := c.endpoints["virtualnic"] + "/"

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
