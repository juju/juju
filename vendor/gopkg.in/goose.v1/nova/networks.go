// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

// Nova api calls for managing networks, which may use either the old
// nova-network code or delegate through to the neutron api.
// See documentation at:
// <http://docs.openstack.org/api/openstack-compute/2/content/ext-os-networks.html>

package nova

import (
	"gopkg.in/goose.v1/client"
	"gopkg.in/goose.v1/errors"
	goosehttp "gopkg.in/goose.v1/http"
)

const (
	apiNetworks = "os-networks"
	// The os-tenant-networks extension is a newer addition aimed at exposing
	// management of networks to unprivileged accounts. Not used at present.
	apiTenantNetworks = "os-tenant-networks"
)

// Network contains details about a labeled network
type Network struct {
	Id    string `json:"id"`    // UUID of the resource
	Label string `json:"label"` // User-provided name for the network range
	Cidr  string `json:"cidr"`  // IP range covered by the network
}

// ListNetworks gives details on available networks
func (c *Client) ListNetworks() ([]Network, error) {
	var resp struct {
		Networks []Network `json:"networks"`
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", apiNetworks, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of networks")
	}
	return resp.Networks, nil
}
