// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// VnicSetParams used as params in CreateVnicSet
type VnicSetParams struct {

	// AppliedAcls is a list of ACLs applied to the VNICs in the set.
	AppliedAcls []string `json:"appliedAcls,omitempty"`

	// Description of the object
	Description string `json:"description,omitempty"`

	// Name is the name of the vnic set
	Name string `json:"name"`

	// Tags associated with the object.
	Tags []string `json:"tags"`

	// List of VNICs associated with this VNIC set
	Vnics []string `json:"vnics"`
}

func (v VnicSetParams) validate() (err error) {
	if v.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty virtual nic set name",
		)
	}

	return nil
}

// CreateVnicSet creates a new virtual nic set
func (c *Client) CreateVnicSet(
	p VnicSetParams,
) (resp response.VnicSet, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["virtualnicset"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		resp: &resp,
		body: &p,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteVnicSet deletes a virtual nic set
func (c *Client) DeleteVnicSet(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty virtual nic set name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["virtualnicset"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// AllVnicSets retrives details of all virtual nic set in the oracle cloud account
func (c *Client) AllVnicSets(filter []Filter) (resp response.AllVnicSets, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["virtualnicset"], c.identify, c.username)

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

// VnicSetDetails retrives details of a virtual nic set
func (c *Client) VnicSetDetails(name string) (resp response.VnicSet, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty virtual nic set name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["virtualnicset"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// UpdateVnicSet changes option, specification, attributes in a vNicSet
func (c *Client) UpdateVnicSet(
	p VnicSetParams,
	currentName string,
) (resp response.VnicSet, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty virtual nic set current name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["virtualnicset"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		resp: &resp,
		body: &p,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
