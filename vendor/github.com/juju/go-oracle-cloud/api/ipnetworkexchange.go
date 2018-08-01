// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// CreateIpNetworkExchange create an IP network exchange to
// enable access between IP networks that have non-overlapping
// addresses, so that instances on these networks can exchange
// packets with each other without NAT.
// After creating an IP network exchange, you can add IP networks
// to the same IP network exchange to enable access between instances
// on these IP networks. Use PUT /network/v1/ipnetwork/{name} request
// to add an IP network to an IP network exchange.
// An IP network exchange can include multiple IP networks,
// but an IP network can be added to only one IP network exchange.
func (c *Client) CreateIpNetworkExchange(
	description string,
	name string,
	tags []string,
) (resp response.IpNetworkExchange, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty ip network exchange name",
		)
	}

	params := struct {
		Description string   `json:"description,omitempty"`
		Name        string   `json:"name"`
		Tags        []string `json:"tags,omitempty"`
	}{
		Description: description,
		Name:        name,
		Tags:        tags,
	}

	url := c.endpoints["ipnetworkexchange"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		body: &params,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteIpNetworkExchange deletes a network exchange given a name
func (c *Client) DeleteIpNetworkExchange(
	name string,
) (err error) {

	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty ip network exchange name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipnetworkexchange"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// AllIpNetworkExchanges retrieves details of all the IP
// network exchanges that are available in the specified container.
func (c *Client) AllIpNetworkExchanges(
	filter []Filter,
) (resp response.AllIpNetworkExchanges, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["ipnetworkexchange"], c.identify, c.username)

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

// IpNetworkExchangeDetails retrieves details of a specific IP network exchange.
func (c *Client) IpNetworkExchangeDetails(
	name string,
) (resp response.IpNetworkExchange, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty ip network exchange name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipnetworkexchange"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
