// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// IpAddressAssociation retrives details of the specified IP address association.
func (c *Client) IpAddressAssociationDetails(name string) (resp response.IpAddressAssociation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip address association name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressassociation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil

}

// AllIpAddressAssociation Retrieves details of the specified IP address association.
func (c *Client) AllIpAddressAssociations(filter []Filter) (resp response.AllIpAddressAssociations, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["ipaddressassociation"], c.identify, c.username)

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

// CreateIpAddressAssociation creates an IP address association
// to associate an IP address reservation, a public IP address,
// with a vNIC of an instance either while creating the instance
// or when an instance is already running.
func (c *Client) CreateIpAddressAssociation(
	description string,
	ipAddressReservation string,
	vnic string,
	name string,
	tags []string,
) (resp response.IpAddressAssociation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip address association name")
	}

	if ipAddressReservation == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip address association reservation")
	}

	if vnic == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip address association vnic")
	}

	// construct the body for the post request
	params := response.IpAddressAssociation{
		IpAddressReservation: ipAddressReservation,
		Vnic:                 vnic,
		Name:                 name,
		Tags:                 tags,
		Description:          description,
	}

	url := c.endpoints["ipaddressassociation"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		resp: &resp,
		body: &params,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteIpAddressAssociation deletes the specified IP address association.
// Ensure that the IP address association is not being used before deleting it.
func (c *Client) DeleteIpAddressAssociation(name string) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty ip address association name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressassociation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// UpdateIpAddressAssociation updates the specified IP address association.
// You can update values for the following parameters
// of an IP address association: description, ipAddressReservation,
// vnic, and tags. If you associate an IP reservation with a vNIC while
// creating or updating the IP address association, then you can remove
// the association between the IP address reservation and vNIC by updating the IP address association.
// However, if you associate an IP reservation with an instance while creating the instance,
// then to remove the IP reservation, update the instance orchestration.
// Otherwise, whenever your instance orchestration is stopped and restarted,
// the IP reservation will again be associated with the vNIC.
func (c *Client) UpdateIpAddressAssociation(
	currentName,
	ipAddressReservation,
	vnic,
	newName string,
) (resp response.IpAddressAssociation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip address association current name")
	}

	if ipAddressReservation == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip address association reservation")
	}

	if vnic == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip address association vnic")
	}

	if newName == "" {
		newName = currentName
	}

	// construct the body for the post request
	params := response.IpAddressAssociation{
		IpAddressReservation: ipAddressReservation,
		Vnic:                 vnic,
		Name:                 newName,
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressassociation"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		resp: &resp,
		body: &params,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
