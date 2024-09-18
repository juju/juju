// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ipaddress

import corenetwork "github.com/juju/juju/core/network"

// ConfigType represents the means via which
// an IP address is configured, as recorded in the
// ip_address_config_type lookup table.
type ConfigType int

const (
	ConfigTypeUnknown ConfigType = iota
	ConfigTypeDHCP
	ConfigTypeDHCPv6
	ConfigTypeSLAAC
	ConfigTypeStatic
	ConfigTypeManual
	ConfigTypeLoopback
)

// MarshallConfigType converts an IP address config type to a db config type id.
func MarshallConfigType(configType corenetwork.AddressConfigType) ConfigType {
	switch configType {
	case corenetwork.ConfigDHCP:
		return ConfigTypeDHCP
	case corenetwork.ConfigStatic:
		return ConfigTypeStatic
	case corenetwork.ConfigManual:
		return ConfigTypeManual
	case corenetwork.ConfigLoopback:
		return ConfigTypeLoopback
	}
	return ConfigTypeUnknown
}

// Scope represents the scope of
// an IP address, as recorded in
// the ip_address_scope lookup table.
type Scope int

const (
	ScopeUnknown Scope = iota
	ScopePublic
	ScopeCloudLocal
	ScopeMachineLocal
	ScopeLinkLocal
)

// MarshallScope converts an address scope to a db scope id.
func MarshallScope(scope corenetwork.Scope) Scope {
	switch scope {
	case corenetwork.ScopePublic:
		return ScopePublic
	case corenetwork.ScopeCloudLocal:
		return ScopeCloudLocal
	case corenetwork.ScopeMachineLocal:
		return ScopeMachineLocal
	case corenetwork.ScopeLinkLocal:
		return ScopeLinkLocal
	}
	return ScopeUnknown
}

// AddressType represents the type of
// an IP address, as recorded in the
// ip_address_type lookup table.
type AddressType int

const (
	AddressTypeIPv4 AddressType = iota
	AddressTypeIPv6
)

// MarshallAddressType converts an address type to a db address type id.
func MarshallAddressType(addressType corenetwork.AddressType) AddressType {
	switch addressType {
	case corenetwork.IPv4Address:
		return AddressTypeIPv4
	case corenetwork.IPv6Address:
		return AddressTypeIPv6
	}
	return AddressTypeIPv4
}

// Origin represents the origin of
// an IP address, as recorded in the
// ip_address_origin lookup table.
type Origin int

const (
	OriginHost Origin = iota
	OriginProvider
)

// MarshallOrigin converts an address origin to a db origin id.
func MarshallOrigin(origin corenetwork.Origin) Origin {
	switch origin {
	case corenetwork.OriginMachine:
		return OriginHost
	case corenetwork.OriginProvider:
		return OriginProvider
	}
	return OriginHost
}
