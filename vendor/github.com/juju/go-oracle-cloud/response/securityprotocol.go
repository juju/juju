// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// SecurityProtocol is a security protocol allows you to specify a transport protocol
// and the source and destination ports to be used with the specified protocol.
// It is used for matching packets in a security rule.
// When you create a security rule, the protocols and ports of the
// specified security protocols are used to determine the type of traffic
// that is permitted by that security rule. If you don't specify protocols
// and ports in a security protocol, traffic is permitted over all protocols and ports.
// You can specify a security protocol in multiple security rules.
// So if you have a protocol that you want to use in a number of
// security rules, you don't have to create the protocol multiple times.
type SecurityProtocol struct {

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

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

type AllSecurityProtocols struct {
	Result []SecurityProtocol `json:"result,omitempty"`
}
