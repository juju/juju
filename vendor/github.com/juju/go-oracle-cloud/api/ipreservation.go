// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// AllIpReservations Retrieves details of the IP reservations that are available
// You can filter by tags, used and permanent.
func (c *Client) AllIpReservations(filter []Filter) (resp response.AllIpReservations, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["ipreservation"], c.identify, c.username)

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

// IpReservationDetails retrieves details of an IP reservation.
// You can use this request to verify whether the
// CreateIpReservation or PutIpReservatio were completed successfully.
func (c *Client) IpReservationDetails(name string) (resp response.IpReservation, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip reservation name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipreservation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// CreateIpReservation creates an IP reservation.
// After creating an IP reservation, you can associate it with
// an instance by using the CrateIpAddressAssociation method
func (c *Client) CreateIpReservation(
	name string,
	parentpool common.IPPool,
	permanent bool,
	tags []string,
) (resp response.IpReservation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty ip reservation name",
		)
	}

	params := struct {
		Permanent  bool          `json:"permanent"`
		Tags       []string      `json:"tags,omitempty"`
		Name       string        `json:"name"`
		Parentpool common.IPPool `json:"parentpool"`
	}{
		Permanent:  permanent,
		Tags:       tags,
		Name:       name,
		Parentpool: parentpool,
	}

	url := c.endpoints["ipreservation"] + "/"

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

// DeleteIpReservation deletes the ip reservation of a instance.
// When you no longer need an IP reservation, you can delete it.
// Ensure that no instance is using the IP reservation that you want to delete.
func (c *Client) DeleteIpReservation(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty ip reservation name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipreservation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}
	return nil
}

// UpdateIpreservation changes the permanent field of an IP reservation
// from false to true or vice versa.
// You can use this command when, for example, you want to delete an
// instance but retain its autoallocated public IP address as a permanent IP
// reservation for use later with another instance. In such a case, before
// deleting the instance, change the permanent field of the IP reservation
// from false to true.
// Note that if you change the permanent field of an IP reservation tofalse,
// and if the reservation is not associated with an instance, then
// the reservation will be deleted.
// You can also update the tags that are used to identify the IP reservation.
func (c *Client) UpdateIpReservation(
	currentName string,
	newName string,
	parentpool common.IPPool,
	permanent bool,
	tags []string,
) (resp response.IpReservation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty ip reservation current name",
		)
	}

	if newName == "" {
		newName = currentName
	}

	params := struct {
		Permanent  bool          `json:"permanent"`
		Tags       []string      `json:"tags,omitempty"`
		Name       string        `json:"name"`
		Parentpool common.IPPool `json:"parentpool"`
	}{
		Permanent:  permanent,
		Tags:       tags,
		Name:       newName,
		Parentpool: parentpool,
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipreservation"], currentName)

	if err = c.request(paramsRequest{
		body: &params,
		url:  url,
		verb: "PUT",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
