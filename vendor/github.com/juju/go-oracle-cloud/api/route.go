// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// RouteParams params that will be passed to the CreateRoute func
type RouteParams struct {

	// Specify common.AdminDistanceZero or common.AdminDistranceOne,
	// or common.AdminDistranceTwo,
	// as the route's administrative distance.
	// If you do not specify a value, the
	// default value is common.AdminDistanceZero,
	// The same prefix can be used in multiple routes.
	// In this case, packets are routed over all the matching routes with
	// the lowest administrative distance. In the case multiple routes with
	// the same lowest administrative distance match,
	// routing occurs over all these routes using ECMP.
	AdminDistance common.AdminDistance `json:"adminDistance,omitempty"`

	// Description is the description of the route
	Description string `json:"description,omitempty,omitempty"`

	// IpAddressPrefix is the IPv4 address prefix, in CIDR format,
	// of the external network (external to the vNIC set)
	// from which you want to route traffic.
	IpAddressPrefix string `json:"ipAddressPrefix"`

	// Name is the name of the route
	Name string `json:"name"`

	// NextHopVnicSet is the name of the virtual NIC set to route matching
	// packets to. Routed flows are load-balanced among all
	// the virtual NICs in the virtual NIC set.
	NextHopVnicSet string `json:"nextHopVnicSet"`

	// Tags associated with the object.
	Tags []string `json:"tags,omitempty"`
}

// validate checks every param is it's valid
func (r RouteParams) validate() (err error) {

	if r.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty route name",
		)
	}

	if r.NextHopVnicSet == "" {
		return errors.New(
			"go-oracle-cloud: Empty route next hop vnic set",
		)
	}

	if r.IpAddressPrefix == "" {
		return errors.New(
			"go-oracle-cloud: Empty route ip address prefix",
		)
	}

	return nil
}

// CreateRoute creates a route, which specifies the IP address
// of the destination as well as a vNICset which provides
// the next hop for routing packets.
func (c *Client) CreateRoute(
	p RouteParams,
) (resp response.Route, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["route"] + "/"

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

// DeleteRoute deletes a route that has the name given in the
// oracle cloud account
func (c *Client) DeleteRoute(
	name string,
) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty route name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["route"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// RouteDetails retrives a route details that has a given name
// from the oracle cloud account
func (c *Client) RouteDetails(
	name string,
) (resp response.Route, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty route name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["route"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllRoutes retrieves details of all the routes
// that are available in the specified container.
func (c *Client) AllRoutes(
	filter []Filter,
) (resp response.AllRoutes, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/", c.endpoints["route"],
		c.identify, c.username)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// UpdateRoute you can update the following parameter values for a route:
// IP address of the destination, vNICset that provides the
// next hop for routing packets, and the route's administrative distance
func (c *Client) UpdateRoute(
	p RouteParams,
	currentName string,
) (resp response.Route, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty route current name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["route"], currentName)

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
