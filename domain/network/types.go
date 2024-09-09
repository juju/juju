// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// SubnetArg represents a subnet to be persisted.
type SubnetArg struct {
	network.SubnetInfo
	CIDRAddressRange
}

// IPAddressBytes represents an IP address as
// unsigned int64 values for msb, lsb.
type IPAddressBytes struct {
	MSB database.Uint64
	LSB database.Uint64
}

// CIDRAddressRange represents a CIDR address range.
type CIDRAddressRange struct {
	Start IPAddressBytes
	End   IPAddressBytes
}

// CIDRAddressRangeFromString parses CIDR string into a CIDRAddressRange.
func CIDRAddressRangeFromString(cidrStr string) (CIDRAddressRange, error) {
	var result CIDRAddressRange
	cidr, err := network.ParseCIDR(cidrStr)
	if err != nil {
		return result, errors.Errorf("invalid subnet CIDR: %w", err)
	}
	start, end := cidr.AddressRange()
	startMSB, startLSB := start.AsInts()
	endMSB, endLSB := end.AsInts()
	result.Start = IPAddressBytes{
		MSB: database.NewUint64(startMSB),
		LSB: database.NewUint64(startLSB),
	}
	result.End = IPAddressBytes{
		MSB: database.NewUint64(endMSB),
		LSB: database.NewUint64(endLSB),
	}
	return result, nil
}

// MustCIDRAddressRangeFromString parses CIDR string into a CIDRAddressRange
// and panics on error.
func MustCIDRAddressRangeFromString(cidrStr string) CIDRAddressRange {
	result, err := CIDRAddressRangeFromString(cidrStr)
	if err != nil {
		panic(err)
	}
	return result
}
