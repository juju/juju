// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// SecIpList a security IP list is a set of IP addresses or subnets external to
// the instances you create in Oracle Compute Cloud Service. These lists
// can then be used as a source when you define security rules.
// Note that, a security IP list named /oracle/public/public-internet
// is predefined in Oracle Compute Cloud Service. You can use this security IP
// list as the source in a security rule to permit traffic from any host on the Internet.
type SecIpList struct {

	// Description is a description of the security IP list.
	Description *string `json:"description,omitempty"`

	// Name is the name of the secure ip list
	Name string `json:"name"`

	// A comma-separated list of the subnets
	// (in CIDR format) or IPv4 addresses for
	// which you want to create this security IP list.
	// For example, to create a security
	// IP list containing the IP addresses
	// 203.0.113.1 and 203.0.113.2, enter one of the following:
	// 203.0.113.0/30
	// 203.0.113.1, 203.0.113.2
	Secipentries []string `json:"secipentries"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// the group id of the secure ip list
	Group_id string `json:"group_id"`

	// the id of the secure ip list
	Id string `json:"id"`
}

// AllSecIpLists holds all the secure ip list entries
// from a given account
type AllSecIpLists struct {
	Result []SecIpList `json:"result,omitempty"`
}
