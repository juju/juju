// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package common

import "errors"

// Networking is a json object of string keys
// Every key is the name of the interface example eth0,eth1, etc.
// And every value is a predefined json objects that holds information
// about the interface
type Networking map[string]Nic

// NicType type representing the
// type of networking card we are dealing with
// a vethernet type or a vnic type
type NicType string

const (
	// Vnic is the networking virtual nic type
	VNic NicType = "vnic"
	// VEthernet is the virtual ethernet type
	VEthernet NicType = "vethernet"
)

// Nic type used to hold information from a
// given interface card
// This wil be used to dump all information from the
// Networking type above
type Nic struct {
	// Dns is the dns of the nic
	Dns []string `json:"dns,omitempty"`

	// Model is the model that has this nic attached
	Model string `json:"model,omitempty"`

	// Nat indicates whether a temporary or permanent
	// public IP address should be assigned
	// to the instance
	Nat string `json:"nat,omitempty"`

	// Seclits is the security lists that you want to add the instance
	Seclists []string `json:"seclists,omitempty"`

	// Vethernet is present if the nic is a vethernet type
	Vethernet string `json:"vethernet,omitempty"`

	// Vnic is present if the nic is a Vnic type
	Vnic string `json:"vnic,omitempty"`

	Ipnetwork string `json:"ipnetwork,omitempty"`
}

func (n Nic) GetType() NicType {
	if n.Vethernet != "" {
		return VEthernet
	}

	return VNic
}

// VcableID is the vcable it of the instance that
// is associated with the ip reservation.
type VcableID string

// Validate checks if the VcableID provided is empty or not
func (v VcableID) Validate() (err error) {
	if v == "" {
		return errors.New("go-oracle-cloud: Empty vcable id")
	}

	return nil
}

// IPPool type describing the
// parent pool of an ip association
type IPPool string

const (
	// PublicIPPool standard ip pool for the oracle provider
	PublicIPPool IPPool = "/oracle/public/ippool"
)

func NewIPPool(name IPPool, prefix IPPrefixType) IPPool {
	return IPPool(prefix) + name
}

type IPPrefixType string

const (
	IPReservationType IPPrefixType = "ipreservation:"
	IPPoolType        IPPrefixType = "ippool:"
)

// IcmpCode is the  ICMP code
// for sec application
type IcmpCode string

const (
	IcmpCodeNetwork  IcmpCode = "network"
	IcmpCodeHost     IcmpCode = "host"
	IcmpCodeProtocol IcmpCode = "protocol"
	IcmpCodePort     IcmpCode = "port"
	IcmpCodeDf       IcmpCode = "df"
	IcmpCodeAdmin    IcmpCode = "admin"
)

// IcmpType is the icmp type for
// sec application
type IcmpType string

const (
	IcmpTypeEcho       IcmpType = "echo"
	IcmpTypeReply      IcmpType = "reply"
	IcmpTypeTTL        IcmpType = "ttl"
	IcmpTypeTraceroute IcmpType = "traceroute"
	IcmpUnreachable    IcmpType = "unreachable"
)

// Protocol is the protocol
// for sec application
type Protocol string

func (p Protocol) Validate() (err error) {
	if p == "" {
		return errors.New(
			"go-oracle-cloud: Empty protocol field",
		)
	}

	return nil
}

const (
	TCP    Protocol = "6"
	ICMP   Protocol = "1"
	IGMP   Protocol = "2"
	IPIP   Protocol = "94"
	RDP    Protocol = "27"
	ESP    Protocol = "50"
	AH     Protocol = "51"
	GRE    Protocol = "47"
	ICMPV6 Protocol = "58"
	OSPF   Protocol = "89"
	PIM    Protocol = "103"
	SCTP   Protocol = "132"
	MPLSIP Protocol = "137"
)

type AdminDistance int

const (
	AdminDistanceZero = 0
	AdminDistanceOne  = 1
	AdminDistanceTwo  = 2
)
