// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// IpNetworkExchange is nn IP network exchange can
// include multiple IP networks, but an IP network
// can be added to only one IP network exchange.
type IpNetworkExchange struct {

	// Description is the description of the ip network exchange
	Description *string `json:"description,omitempty"`

	// Name si the name of the ip network exchange
	Name string `json:"name"`

	// Tags associated with the object.
	Tags []string `json:"tags,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllIpNetworkExchanges is holds all internal ip network exchanges
// from a cloud account
type AllIpNetworkExchanges struct {
	// Result slice of all the ip networks exchanges
	Result []IpNetworkExchange `json:"result, omitempty"`
}
