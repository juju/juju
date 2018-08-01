// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// SecurityRule is a  security rule permits traffic from a specified
// source or to a specified destination. You must specify the direction
// of a security rule - either ingress or egress. In addition, you
// can specify the source or destination of permitted traffic, and
// the security protocol and port used to send or receive packets.
// Each of the parameters that you specify in a security rule provides
// a criterion that the type of traffic permitted by that rule must match.
// Only packets that match all of the specified criteria are permitted.
// If you don't specify match criteria in the security rule, all traffic in
// the specified direction is permitted. The primary function of security
// rules is to help identify the type of traffic to be allowed in the IP network.
type SecurityRule struct {
	// Acl is the name of the acl that contains this rule
	Acl string `json:"acl"`

	// Description is the description of the object
	Description string `json:"description,omitempty"`

	// DstIpAddressPrefixSets list of IP address prefix set names
	// to match the packet's destination IP address.
	DstIpAddressPrefixSets []string `json:"dstIpAddressPrefixSets"`

	// DstVnicSet the name of virtual NIC set containing the
	// packet's destination virtual NIC.
	DstVnicSet string `json:"dstVnicSet"`

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
	SrcIpAddressPrefixSets []string `json:"srcIpAddressPrefixSets"`

	// SrcVnicSet is the name of virtual NIC set containing
	// the packet's source virtual NIC.
	SrcVnicSet string `json:"srcVnicSet"`

	// Tags associated with the object.
	Tags []string `json:"tags,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

type AllSecurityRules struct {
	Result []SecurityRule `json:"result,omitempty"`
}
