// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// AllIpAssociation retrieves the names of objects and subcontainers
// that you can access in the specified container.
// You can filter by parentpool, reservation and vcable
func (c *Client) AllIpAssociations(filter []Filter) (resp response.AllIpAssociations, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["ipassociation"], c.identify, c.username)

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

// IpAssociationDetails retrieves details of the IP associations that are
// available in the specified container
func (c *Client) IpAssociationDetails(name string) (resp response.IpAssociation, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip association name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipassociation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// Creates an association between an IP address
// and the vcable ID of an instance.
func (c *Client) CreateIpAssociation(
	parentpool common.IPPool,
	vcable common.VcableID,
) (resp response.IpAssociation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = vcable.Validate(); err != nil {
		return resp, err
	}

	params := struct {
		Parentpool common.IPPool   `json:"parentpool"`
		Vcable     common.VcableID `json:"vcable"`
	}{
		Parentpool: parentpool,
		Vcable:     vcable,
	}

	url := c.endpoints["ipassociation"] + "/"

	if err = c.request(paramsRequest{
		body: &params,
		url:  url,
		verb: "POST",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// Deletes the specified IP association with the name
func (c *Client) DeleteIpAssociation(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty ip association name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipassociation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}
