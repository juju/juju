// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

type SecApplicationParams struct {

	// Description is a description of the security application.
	Description string `json:"description,omitempty"`

	// Dport is the TCP or UDP destination port number.
	// You can also specify a port range, such as 5900-5999 for TCP.
	// If you specify tcp or udp as the protocol, then the dport
	// parameter is required; otherwise, it is optional.
	// This parameter isn't relevant to the icmp protocol.
	// Note: This request fails if the range-end is lower than the range-start.
	// For example, if you specify the port range as 5000-4000.
	Dport string `json:"dport,omitempty"`

	// Icmpcode is the ICMP code.
	// This parameter is relevant only if you specify
	// icmp as the protocol. You can specify one of the following values:
	//
	// common.IcmpCodeNetwork
	// common.IcmpCodeHost
	// common.IcmpCodeProtocol
	// common.IcmpPort
	// common.IcmpCodeDf
	// common.IcmpCodeAdmin
	//
	// If you specify icmp as the protocol and don't
	// specify icmptype or icmpcode, then all ICMP packets are matched.
	Icmpcode common.IcmpCode `json:"icmpcode,omitempty"`

	// Icmptype
	// The ICMP type. This parameter is relevant only if you specify icmp
	// as the protocol. You can specify one of the following values:
	//
	// common.IcmpTypeEcho
	// common.IcmpTypeReply
	// common.IcmpTypeTTL
	// common.IcmpTraceroute
	// common.IcmpUnreachable
	// If you specify icmp as the protocol and
	// don't specify icmptype or icmpcode, then all ICMP packets are matched.
	Icmptype common.IcmpType `json:"icmptype,omitempty"`

	// Name is the name of the secure application
	Name string `json:"name"`

	// Protocol is the protocol to use.
	// The value that you specify can be either a text representation of
	// a protocol or any unsigned 8-bit assigned protocol number
	// in the range 0-254. See Assigned Internet Protocol Numbers.
	// For example, you can specify either tcp or the number 6.
	// The following text representations are allowed:
	// tcp, udp, icmp, igmp, ipip, rdp, esp, ah, gre, icmpv6, ospf, pim, sctp, mplsip, all.
	// To specify all protocols, set this to all.
	Protocol common.Protocol `json:"protocol"`
}

func (s SecApplicationParams) validate() (err error) {
	if s.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty secure application name",
		)
	}

	if err = s.Protocol.Validate(); err != nil {
		return err
	}

	return nil
}

// CreateSecApplication creates a security application.
// After creating security applications
func (c *Client) CreateSecApplication(p SecApplicationParams) (resp response.SecApplication, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["secapplication"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		body: &p,
		verb: "POST",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteSecApplication deletes a security application. No response is returned.
func (c *Client) DeleteSecApplication(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty secure application name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["secapplication"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// SecApplicationDetails retrieve details of a security application
func (c *Client) SecApplicationDetails(name string) (resp response.SecApplication, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure application name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["secapplication"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllSecApplications retrieves details of the security applications that are in the specified container
func (c *Client) AllSecApplications(filter []Filter) (resp response.AllSecApplications, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["secapplication"], c.identify, c.username)

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

// DefaultSecApplications retrieves details of the default security applications that are
// defined in the cloud. The Oracle cloud defines a number of pre defined rules that
// can be used
func (c *Client) DefaultSecApplications(filter []Filter) (resp response.AllSecApplications, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/oracle/public/",
		c.endpoints["secapplication"])

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
