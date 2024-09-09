// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"encoding/binary"
	"net/netip"
)

// IPAddress wraps a [netip.Addr] instance and provides a
// method to decompose the address into msb, lsb uint64 values.
type IPAddress struct {
	netip.Addr
}

// AsInts returns the MSB and LSB uint64 values for the specified address.
func (a IPAddress) AsInts() (msb uint64, lsb uint64) {
	addrB := a.AsSlice()
	if a.Is4() {
		lsb = uint64(binary.BigEndian.Uint32(addrB[:4]))
	} else {
		msb = binary.BigEndian.Uint64(addrB[:8])
		lsb = binary.BigEndian.Uint64(addrB[8:])
	}
	return msb, lsb
}
