// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// IpAddressPrefixSet is an IP address prefix set lists IPv4 addresses in
// the CIDR address prefix format. After creating an IP address
// prefix set, you can specify it as a source or destination for
// permitted traffic while creating a security rule. See Add a Security Rule.
type IpAddressPrefixSet struct {

	// Description is a description of the ip address prefix set
	Description *string `json:"description,omitmepty"`

	IpAddressPrefixes []string `json:"ipAddressPrefixes"`
	// IpAddressPrefixes is a list of CIDR IPv4 prefixes assigned in the virtual network.

	// Name is the name of the ip address prefix set
	Name string `json:"name"`

	// Tags is strings that you can use to tag the IP address prefix set.
	Tags []string `json:"tags,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllIpAddressPrefixSets holds all ip address prefix sets in the oracle
// cloud account
type AllIpAddressPrefixSets struct {
	// Result internal ip address prefix sets slice
	Result []IpAddressPrefixSet `json:"result,omitempty"`
}
