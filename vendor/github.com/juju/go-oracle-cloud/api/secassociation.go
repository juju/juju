// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// CreateSecAssociation adds an instance to a security list.
func (c *Client) CreateSecAssociation(
	name string,
	seclist string,
	vcable common.VcableID,
) (resp response.SecAssociation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure association name")
	}

	if err := vcable.Validate(); err != nil {
		return resp, err
	}

	if seclist == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure association sec list")
	}

	url := c.endpoints["secassociation"] + "/"

	params := struct {
		Name    string          `json:"name"`
		Vcable  common.VcableID `json:"vcable"`
		Seclist string          `json:"seclist"`
	}{
		Name:    name,
		Vcable:  vcable,
		Seclist: seclist,
	}

	if err = c.request(paramsRequest{
		url:  url,
		body: &params,
		verb: "POST",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteSecAssociaton deletes the specified security association.
// After you delete a security association, it takes a few
// minutes for the change to take effect.
func (c *Client) DeleteSecAssociation(
	name string,
) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty secure association name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["secassociation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// SecAssociationDetails retrieves details about the specified security association
func (c *Client) SecAssociationDetails(
	name string,
) (resp response.SecAssociation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure association name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["secassociation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllSecAssociations retrives all security associations in the oracle cloud account
func (c *Client) AllSecAssociations(filter []Filter) (resp response.AllSecAssociations, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["secassociation"], c.identify, c.username)

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

func (c *Client) AllSecAssociationNames() (resp response.DirectoryNames, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s",
		c.endpoints["secassociation"], c.identify, c.username)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil

}
