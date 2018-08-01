// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// AllIpNetworks retrieves details of all the IP networks
// that are available in the specified container.
func (c *Client) AllIpNetworks(filter []Filter) (resp response.AllIpNetworks, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["ipnetwork"], c.identify, c.username)

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

// IpNetworkDetails retrives details of a an IP network
// that is available in the oracle account
func (c *Client) IpNetworkDetails(name string) (resp response.IpNetwork, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip network name")
	}

	url := fmt.Sprintf("%s%s",
		c.endpoints["ipnetwork"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// CreateIpNetwork creates an IP network. An IP network allows you to define
// an IP subnet in your account. With an IP network you can isolate
// instances by creating separate IP networks and adding instances
// to specific networks. Traffic can flow between instances within
// the same IP network, but by default each network is isolated
// from other networks and from the public Internet.
func (c *Client) CreateIpNetwork(
	description string,
	ipAddressPrefix string,
	ipNetworkExchange string,
	name string,
	publicNaptEnabledFlag bool,
	tags []string,
) (resp response.IpNetwork, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip network name")
	}

	if ipAddressPrefix == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip network address prefix")
	}

	url := c.endpoints["ipnetwork"] + "/"

	params := response.IpNetwork{
		Description:       &description,
		IpAddressPrefix:   ipAddressPrefix,
		IpNetworkExchange: &ipNetworkExchange,
		Name:              name,
		Tags:              tags,
		PublicNaptEnabledFlag: publicNaptEnabledFlag,
	}

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		resp: &resp,
		body: params,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteIpNetwork deletes an IP network with a given name
func (c *Client) DeleteIpNetwork(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty ip network name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipnetwork"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// UpdateIpNetwork can update an IP network and change the specified IP address prefix
// for the network after you've created the network and attached instances to it.
// However, when you change an IP address prefix, it could cause the IP addresses
//currently assigned to existing instances to fall outside the specified IP network.
// If this happens, all traffic to and from those vNICs will be dropped.
// If the IP address of an instance is dynamically allocated, stopping the instance
// orchestration and restarting it will reassign a valid IP address from the IP network to the instance.
// However, if the IP address of an instance is static - that is, if the IP address
// is specified in the instance orchestration while creating the instance - then
// the IP address can't be updated by stopping the instance orchestration
// and restarting it. You would have to manually update the orchestration to assign
// a valid IP address to the vNIC attached to that IP network.
// It is therefore recommended that if you update an IP network, you
// only expand the network by specifying the same IP address prefix
// but with a shorter prefix length. For example, you can expand 192.168.1.0/24
// to 192.168.1.0/20. Don't, however, change the IP address.
// This ensures that all IP addresses that have been currently allocated
// to instances remain valid in the updated IP network.
func (c *Client) UpdateIpNetwork(
	currentName string,
	newName string,
	description string,
	ipNetworkExchange string,
	ipAddressPrefix string,
	publicNaptEnabledFlag bool,
	tags []string,
) (resp response.IpNetwork, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip network current name")
	}

	if ipAddressPrefix == "" {
		return resp, errors.New("go-oracle-cloud: Empty ip network address prefix")
	}

	if newName == "" {
		newName = currentName
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipnetwork"], currentName)

	params := response.IpNetwork{
		Description:       &description,
		IpAddressPrefix:   ipAddressPrefix,
		IpNetworkExchange: &ipNetworkExchange,
		Name:              newName,
		Tags:              tags,
		PublicNaptEnabledFlag: publicNaptEnabledFlag,
	}

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		resp: &resp,
		body: params,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
