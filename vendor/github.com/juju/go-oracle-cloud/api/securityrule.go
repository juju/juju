// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

type SecurityRuleParams struct {
	// Acl is the name of the acl that contains this rule
	Acl string `json:"acl,omitempty"`

	// Description is the description of the object
	Description string `json:"description,omitempty"`

	// DstIpAddressPrefixSets list of IP address prefix set names
	// to match the packet's destination IP address.
	DstIpAddressPrefixSets []string `json:"dstIpAddressPrefixSets,omitmepty"`

	// DstVnicSet the name of virtual NIC set containing the
	// packet's destination virtual NIC.
	DstVnicSet string `json:"dstVnicSet,omitempty"`

	// EnabledFlag false indicated that the security rule is disabled
	EnabledFlag bool `json:"enabledFlag"`

	// FlowDirection is the direction of the flow;
	// Can be "egress" or "ingress".
	FlowDirection common.FlowDirection `json:"flowDirection"`

	// Name is the name of the security rule
	Name string `json:"name"`

	// SecProtocols is the list of security protocol object
	// names to match the packet's protocol and port.
	SecProtocols []string `json:"secProtocols"`

	// SrcIpAddressPrefixSets list of multipart names of
	// IP address prefix set to match the packet's source IP address.
	SrcIpAddressPrefixSets []string `json:"srcIpAddressPrefixSets,omitempty"`

	// SrcVnicSet is the name of virtual NIC set containing
	// the packet's source virtual NIC.
	SrcVnicSet string `json:"srcVnicSet,omitempty"`

	// Tags associated with the object.
	Tags []string `json:"tags,omitempty"`
}

func (s SecurityRuleParams) validate() (err error) {
	if s.Name == "" {
		return errors.New("go-oracle-cloud: Empty security rule name")
	}

	if s.FlowDirection == "" {
		return errors.New("go-oracle-cloud: Empty flow direction in security rule")
	}

	return nil
}

// CreateSecurityRule adds a security rule. A security rule permits traffic
// from a specified source or to a specified destination.
// You must specify the direction of a security rule - either
// ingress or egress. In addition, you can specify the source or
// destination of permitted traffic, and the security protocol and port
// used to send or receive packets. Each of the parameters that you
// specify in a security rule provides a criterion that the
// type of traffic permitted by that rule must match.
//
//
// Only packets that match all of the specified criteria
// are permitted. If you don't specify match criteria in the security rule,
// all traffic in the specified direction is permitted.
//
//
// When you create a security rule with a specified direction,
// say ingress, you should also create a corresponding security rule for the
// opposite direction - in this case, egress.
// This is generally required to ensure that when traffic is permitted in
// one direction, responses or acknowledgement packets in the opposite
// direction are also permitted.
//
// When you create a security rule, you specify the ACL that it belongs to.
// ACLs apply to vNICsets. You can apply multiple ACLs to a vNICset and
// you can apply each ACL to multiple vNICsets. When an ACL is applied to
// a vNICset, every security rule that belongs to the ACL applies to every
// vNIC that is specified in the vNICset.
//
// A security rule allows you to specify the following parameters:
// * The flow direction - ingress or egress
// * (Optional) A source vNICset or a list of source IP address prefix sets, or both
// * (Optional) A destination vNICset or a list of destination
// IP address prefix sets, or both
// * (Optional) A list of security protocols
// * (Optional) The name of the ACL that contains this rule
// * (Optional) An option to disable the security rule
func (c *Client) CreateSecurityRule(
	p SecurityRuleParams,
) (resp response.SecurityRule, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["securityrule"] + "/"

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

// DeteleSecurityRule deletes the specified security rule.
// Before deleting a security rule, ensure that it is not being used.
func (c *Client) DeleteSecurityRule(
	name string,
) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty security rule name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["securityrule"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// SecurityRuleDetais retrieves details of the specified security rule.
func (c *Client) SecurityRuleDetails(
	name string,
) (resp response.SecurityRule, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty security rule name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["securityrule"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllSecurityRules retrieves details of all the security rules
// in the specified container.
func (c *Client) AllSecurityRules(filter []Filter) (resp response.AllSecurityRules, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["securityrule"], c.identify, c.username)

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

// UpdateSecurityRule you can update values of all the parameters of a security
// rule except the name. You can also disable a security rule,
// by setting the value of the enabledFlag parameter as false.
func (c *Client) UpdateSecurityRule(
	p SecurityRuleParams,
) (resp response.SecurityRule, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := fmt.Sprintf("%s%s", c.endpoints["securityrule"], p.Name)

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
