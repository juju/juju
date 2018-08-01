// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

type SecurityProtocolParams struct {

	// Description is a description of the security protocol
	Description string `json:"description,omitempty"`

	// DstPortSet enter a list of port numbers or port range strings.
	// Traffic is enabled by a security rule when a packet's destination
	// port matches the ports specified here.
	// For TCP, SCTP, and UDP, each port is a destination transport port,
	// between 0 and 65535, inclusive. For ICMP,
	// each port is an ICMP type, between 0 and 255, inclusive.
	// If no destination ports are specified, all destination ports or
	// ICMP types are allowed.
	DstPortSet []string `json:"dstPortSet"`

	// IpProtocol the protocol used in the data portion of the IP datagram.
	// Specify one of the permitted values or enter a number in the range 0–254
	// to represent the protocol that you want to specify. See Assigned Internet
	// Protocol Numbers. Permitted values are: tcp, udp, icmp, igmp, ipip,
	// rdp, esp, ah, gre, icmpv6, ospf, pim, sctp, mplsip, all.
	// Traffic is enabled by a security rule when the protocol in the packet
	// matches the protocol specified here. If no protocol is specified,
	// all protocols are allowed.
	IpProtocol common.Protocol `json:"ipProtocol"`

	// Name is the name of the security protocol
	Name string `json:"name"`

	// SrcPortSet is a list of port numbers or port range strings.
	// Traffic is enabled by a security rule when a packet's source port
	// matches the ports specified here.
	// For TCP, SCTP, and UDP, each port is a source transport port,
	// between 0 and 65535, inclusive. For ICMP, each port is an ICMP type,
	// between 0 and 255, inclusive.
	// If no source ports are specified, all source ports or ICMP
	// types are allowed.
	SrcPortSet []string `json:"srcPortSet"`

	// Tags is strings that you can use to tag the security protocol.
	Tags []string
}

func (s SecurityProtocolParams) validate() (err error) {
	if s.Name == "" {
		return errors.New("go-oracle-cloud: Empty security protocol name")
	}

	return nil
}

// CreateSecurityProtocol creates a security protocol. A security protocol
// allows you to specify a transport protocol and the source and destination
// ports to be used with the specified protocol. When you create
// a security rule, the protocols and ports of the specified security
// protocols are used to determine the type of traffic that is permitted by
// that security rule. If you don't specify protocols and ports in a
// security protocol, traffic is permitted over all protocols and ports.
func (c *Client) CreateSecurityProtocol(
	p SecurityProtocolParams,
) (resp response.SecurityProtocol, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["securityprotocol"] + "/"

	if err = c.request(paramsRequest{
		verb: "POST",
		url:  url,
		body: &p,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteSecurityProtocol deletes a security protocol.
// Ensure that the security protocol is not being used before deleting it.
func (c *Client) DeleteSecurityProtocol(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty security protocol name",
		)
	}
	url := fmt.Sprintf("%s%s", c.endpoints["securityprotocol"], name)

	if err = c.request(paramsRequest{
		verb: "DELETE",
		url:  url,
	}); err != nil {
		return err
	}

	return nil
}

// AllSecurityProtocols retrieve details of all security
// protocols in the specified container.
func (c *Client) AllSecurityProtocols(filter []Filter) (resp response.AllSecurityProtocols, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["securityprotocol"], c.identify, c.username)

	if err = c.request(paramsRequest{
		verb:   "GET",
		url:    url,
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// SecurityProtocol retrieves details of the specified security protocol.
func (c *Client) SecurityProtocolDetails(
	name string,
) (resp response.SecurityProtocol, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty security protocol name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["securityprotocol"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// UpdateSecurityProtocol update an existing security protocol.
// You can update values of the description, ipProtocol, srcPortSet,
// dstPortSet, and tags parameters of a security protocol.
func (c *Client) UpdateSecurityProtocol(
	p SecurityProtocolParams,
) (resp response.SecurityProtocol, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := fmt.Sprintf("%s%s", c.endpoints["securityprotocol"], p.Name)

	if err = c.request(paramsRequest{
		verb: "PUT",
		url:  url,
		body: &p,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
