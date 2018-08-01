// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// IpAddressReservationParams
type IpAddressReservationParams struct {

	// Description is the description of the ip address reservation
	Description string `json:"description,omitmepty"`

	// IpAddressPool is the IP address pool from which you want
	// to reserve an IP address. Enter one of the following:
	// * /oracle/public/public-ippool: When you attach an IP address
	// from this pool to an instance, you enable
	// access between the public Internet and the instance.
	// * /oracle/public/cloud-ippool: When you attach
	// an IP address from this pool to an instance, the instance
	// can communicate privately (that is, without traffic going over
	// the public Internet) with other Oracle Cloud services, such as the
	// TODO(sgiulitti) more research on this type
	IpAddressPool common.IPPool `json:"ipAddressPool,omitempty"`

	// Name is the name of the ip address reservation
	Name string `json:"name"`

	// Tags is the strings that you can use to tag the IP address reservation.
	Tags []string `json:"tags,omitempty"`
}

// validate will validate the ip address reservation params
func (i IpAddressReservationParams) validate() (err error) {
	if i.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty ip address reservation name",
		)
	}

	return nil
}

// CreateIpAddressReservation creates a reserves a NAT IPv4 address,
// which you can associate with one or more virtual NICs for routing
// traffic outside an IP network or an IP network exchange using NAT.
// If you want an interface on your instances to be accessible from
// the public Internet, or if you want your instances to be able
// to communicate with other Oracle services on other IP networks,
// create an IP address reservation to reserve a public IP address,
// and then associate that reserved IP address with a vNIC on your instance.
// You can reserve a public IP address from one of two IP pools:
// /oracle/public/public-ippool: When you attach an IP address from this pool to an instance,
// you enable access between the public Internet and the instance.
//
// /oracle/public/cloud-ippool: When you attach an IP address from
// this pool to an instance, the instance can communicate privately
// (that is, without traffic going over the public Internet) with other
// Oracle Cloud services, such as the REST endpoint of an
// Oracle Storage Cloud Service account in the same region.
//
// A public IP address or a cloud IP address can be associated
// with only one vNIC at a time. However,
// a single vNIC can have a maximum of two NAT IP addresses, one from each IP pool.
func (c *Client) CreateIpAddressReservation(
	p IpAddressReservationParams,
) (resp response.IpAddressReservation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["ipaddressreservation"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		body: &p,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// UpdateIpAddressReservation updates the specified IP address reservation.
// Updates the description and tags of the specified IP address reservation.
func (c *Client) UpdateIpAddressReservation(
	p IpAddressReservationParams,
	currentName string,
) (resp response.IpAddressReservation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty ip address reservation current name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressreservation"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		body: &p,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteIpAddressReservation deletes the specified IP address reservation.
// Ensure that the IP reservation that you want to delete isn't associated with a vNIC.
func (c *Client) DeleteIpAddressReservation(
	name string,
) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty ip address reservation name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressreservation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// IpAddressReservationDetails retrieves details of the specified IP address reservation.
func (c *Client) IpAddressReservationDetails(
	name string,
) (resp response.IpAddressReservation, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty ip address reservation name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressreservation"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllIpAddressReservations retrieves details of the IP address reservations
// that are available in the specified container.
func (c *Client) AllIpAddressReservations(
	filter []Filter,
) (resp response.AllIpAddressReservations, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["ipaddressreservation"], c.identify, c.username)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
