// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ipaddress

import corenetwork "github.com/juju/juju/core/network"

// ConfigType represents the means via which an IP address is
// configured, as recorded in the ip_address_config_type lookup table.
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

// UnMarshallConfigType converts db config type id to an IP address config type.
func UnMarshallConfigType(configType ConfigType) corenetwork.AddressConfigType {
	switch configType {
	case ConfigTypeDHCP:
		return corenetwork.ConfigDHCP
	case ConfigTypeStatic:
		return corenetwork.ConfigStatic
	case ConfigTypeManual:
		return corenetwork.ConfigManual
	case ConfigTypeLoopback:
		return corenetwork.ConfigLoopback
	}
	return corenetwork.ConfigUnknown
}

// Scope represents the scope of an IP address, as recorded in
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

// UnMarshallScope converts a db scope id to an IP address scope.
func UnMarshallScope(scope Scope) corenetwork.Scope {
	switch scope {
	case ScopePublic:
		return corenetwork.ScopePublic
	case ScopeCloudLocal:
		return corenetwork.ScopeCloudLocal
	case ScopeMachineLocal:
		return corenetwork.ScopeMachineLocal
	case ScopeLinkLocal:
		return corenetwork.ScopeLinkLocal
	}
	return corenetwork.ScopeUnknown
}

// AddressType represents the type of an IP address, as recorded in
// the ip_address_type lookup table.
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

// UnMarshallAddressType converts a db address type id to an IP address type.
func UnMarshallAddressType(addressType AddressType) corenetwork.AddressType {
	switch addressType {
	case AddressTypeIPv4:
		return corenetwork.IPv4Address
	case AddressTypeIPv6:
		return corenetwork.IPv6Address
	}
	return corenetwork.IPv4Address
}

// Origin represents the origin of an IP address, as recorded in
// the ip_address_origin lookup table.
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

// UnMarshallOrigin converts a db origin id to an IP address origin.
func UnMarshallOrigin(origin Origin) corenetwork.Origin {
	switch origin {
	case OriginHost:
		return corenetwork.OriginMachine
	case OriginProvider:
		return corenetwork.OriginProvider
	}
	return corenetwork.OriginMachine
}
