// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import (
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
)

type SecApplication struct {

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
	Protocol common.Protocol

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// Not in the documentation, but the API returns Value1 which holds the port range start
	// Should coincide with Dport if only one port is opened
	Value1 int `json:"value1"`

	// Not in the documentation, but the API returns Value2 which holds the port range end
	// If only one port is opened, this value will hold the value -1. If multiple ports are
	// opened, this value will hold the second element (delimited by - ) from Dport
	Value2 int `json:"value2"`

	// Not in the documentation. No idea what this is. My crystal ball is broken.
	Id string `json:"id"`
}

func (s *SecApplication) String() string {
	return s.Name
}

func (s *SecApplication) PortProtocolPair() string {
	if s.Dport == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", s.Dport, s.Protocol)
}

type AllSecApplications struct {
	Result []SecApplication `json:"result,omitempty"`
}
