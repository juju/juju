// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// VpnEndpoint you can create secure IPSec-based tunnels between your data
// center and the instances in your Oracle Compute Cloud
// Service site to securely access your instances.
// A vpnendpoint object represents a VPN tunnel to
// your Oracle Compute Cloud Service site. You can
// create up to 20 VPN tunnels to your Oracle Compute
// Cloud Service site. You can use any internet service
// provider to access your Oracle Compute Cloud Service
// site, provided you have a VPN device to terminate an IPSec VPN tunnel

type VpnEndpoint struct {
	// Customer_vpn_gateway specify the IP address of the
	// VPN gateway in your data center through which
	// you want to connect to the Oracle Cloud VPN gateway.
	// Your gateway device must support route-based VPN
	// and IKE (Internet Key Exchange) configuration
	// using pre-shared keys.
	Customer_vpn_gateway string `json:"customer_vpn_gateway"`

	// Enabled flag, enables the VPN endpoint.
	// To start a VPN connection, set to true.
	// A connection is established immediately, if possible.
	// If you do not specify this option, the VPN
	// endpoint is disabled and the
	// connection is not established
	Enabled bool `json:"enabled"`

	// Name is the name of the vpn endpoint resource
	Name string `json:"name"`

	// Psk is the pre-shared VPN key. Enter the pre-shared key.
	// This must be the same key that you provided
	// when you requested the service. This secret key
	// is shared between your network gateway and the
	// Oracle Cloud network for authentication.
	// Specify the full path and name of the text file
	// that contains the pre-shared key. Ensure
	// that the permission level of the text file is
	// set to 400. The pre-shared VPN key must not
	// exceed 256 characters.
	Psk string `json:"psk"`

	// Reachable_routes is a list of routes (CIDR prefixes)
	// that are reachable through this VPN tunnel.
	// You can specify a maximum of 20 IP subnet addresses.
	// Specify IPv4 addresses in dot-decimal
	// notation with or without mask.
	Reachable_routes []string `json:"reachable_routes"`

	// Status is the current status of the VPN tunnel.
	Status string `json:"status"`

	// Status_desc describes the current status of the VPN tunnel.
	Status_desc string `json:"status_desc"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

type AllVpnEndpoints struct {
	Result []VpnEndpoint `json:"result,omitmepty"`
}
