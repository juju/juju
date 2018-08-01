// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

type VpnEndpointParams struct {
	// Customer_vpn_gateway specify the IP address of the
	// VPN gateway in your data center through which
	// you want to connect to the Oracle Cloud VPN gateway.
	// Your gateway device must support route-based VPN
	// and IKE (Internet Key Exchange) configuration
	// using pre-shared keys.
	Customer_vpn_gateway string `json:"customer_vpn_gateway"`

	// Enabled flag, enables the VPN endpoint.
	// To start a VPN connection, set to true.
	// A connection is established immediately, if possible.
	// If you do not specify this option, the VPN
	// endpoint is disabled and the
	// connection is not established
	Enabled bool `json:"enabled"`

	// Name is the name of the vpn endpoint resource
	Name string `json:"name"`

	// Psk is the pre-shared VPN key. Enter the pre-shared key.
	// This must be the same key that you provided
	// when you requested the service. This secret key
	// is shared between your network gateway and the
	// Oracle Cloud network for authentication.
	// Specify the full path and name of the text file
	// that contains the pre-shared key. Ensure
	// that the permission level of the text file is
	// set to 400. The pre-shared VPN key must not
	// exceed 256 characters.
	Psk string `json:"psk"`

	// Reachable_routes is a list of routes (CIDR prefixes)
	// that are reachable through this VPN tunnel.
	// You can specify a maximum of 20 IP subnet addresses.
	// Specify IPv4 addresses in dot-decimal
	// notation with or without mask.
	Reachable_routes []string `json:"reachable_routes"`
}

func (v VpnEndpointParams) validate() (err error) {
	if v.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty vpn endpoint name",
		)
	}

	if v.Customer_vpn_gateway == "" {
		return errors.New(
			"go-oracle-cloud: Empty vpn endpoint customer gateway",
		)
	}

	if v.Psk == "" {
		return errors.New(
			"go-oracle-cloud: Empty vpn enpoint pre shared vpn key",
		)
	}

	if v.Reachable_routes == nil || len(v.Reachable_routes) == 0 {
		return errors.New(
			"go-oracle-cloud: Empty vpn endpoint reachable routes",
		)
	}

	return nil
}

// CreateVpnEndpoint creates a VPN tunnel between
// your data center and your Oracle Compute
// Cloud Service site. You can create up to 20 VPN
// tunnels to your Oracle Compute Cloud Service site
func (c *Client) CreateVpnEndpoint(
	p VpnEndpointParams,
) (resp response.VpnEndpoint, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["vpnendpoint"] + "/"

	if err = c.request(paramsRequest{
		verb: "POST",
		resp: &resp,
		body: &p,
		url:  url,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

func (c *Client) DeleteVpnEndpoint(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty vpn endpoint name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["vpnendpoint"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

func (c *Client) VpnEndpointDetails(
	name string,
) (resp response.VpnEndpoint, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty vpn endpoint name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["vpnendpoint"], name)

	if err = c.request(paramsRequest{
		url:  url,
		resp: &resp,
		verb: "GET",
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

func (c *Client) AllVpnEndpoints(
	filter []Filter,
) (resp response.AllVpnEndpoints, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["vpnendpoint"], c.identify, c.username)

	if err = c.request(paramsRequest{
		url:    url,
		filter: filter,
		verb:   "GET",
		resp:   &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
