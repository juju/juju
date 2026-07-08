// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ipfamily

import "github.com/juju/juju/internal/errors"

// IPFamily represents the IP address family constraint for a machine's
// network interfaces.
type IPFamily string

const (
	// IPv4 restricts the machine to IPv4-only networking.
	IPv4 IPFamily = "ipv4"

	// IPv6 restricts the machine to IPv6-only networking.
	IPv6 IPFamily = "ipv6"

	// Dual allows both IPv4 and IPv6 networking (dual-stack).
	Dual IPFamily = "dual"
)

// String implements the Stringer interface.
func (f IPFamily) String() string {
	return string(f)
}

// ParseIPFamily parses a string into an IPFamily, returning an error for
// empty or unrecognised values.
func ParseIPFamily(s string) (IPFamily, error) {
	f := IPFamily(s)
	return f, f.Validate()
}

// Validate returns an error if the IPFamily is not one of the recognised
// values.
func (f IPFamily) Validate() error {
	switch f {
	case IPv4, IPv6, Dual:
		return nil
	case "":
		return errors.Errorf("must be one of %q, %q, %q", IPv4, IPv6, Dual)
	default:
		return errors.Errorf("%q not recognized; valid values are %q, %q, %q", f, IPv4, IPv6, Dual)
	}
}
