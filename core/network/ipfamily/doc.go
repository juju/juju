// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package ipfamily defines the IPFamily type used as the ip-family constraint
// value for machines and applications.
//
// IPFamily is a string enumeration with three valid values -- IPv4 ("ipv4"),
// IPv6 ("ipv6"), and Dual ("dual") -- that restrict the IP addressing mode of
// a machine's network interfaces. ParseIPFamily and Validate enforce that only
// these three values are accepted; the zero value (empty string) is explicitly
// invalid.
//
// See github.com/juju/juju/core/constraints for how IPFamily is embedded in
// the constraint Value type. See github.com/juju/juju/domain/constraints for
// the domain-layer Constraints struct that carries IPFamily to and from state.
package ipfamily
