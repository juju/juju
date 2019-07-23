// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"math/rand"
)

// Address describes a network address.
type Address struct {
	Value     string
	Type      string
	Scope     string
	SpaceName string
	// TODO (manadart 2019-07-12): Rename to ProviderSpaceId for consistency.
	SpaceProviderId string
}

// macAddressTemplate is suitable for generating virtual MAC addresses,
// particularly for use by container devices.
// The last 3 segments are randomised.
// TODO (manadart 2018-06-21) Depending on where this is utilised,
// ensuring MAC address uniqueness within a model might be prudent.
const macAddressTemplate = "00:16:3e:%02x:%02x:%02x"

// GenerateVirtualMACAddress creates a random MAC address within the address
// space implied by macAddressTemplate above.
var GenerateVirtualMACAddress = func() string {
	digits := make([]interface{}, 3)
	for i := range digits {
		digits[i] = rand.Intn(256)
	}
	return fmt.Sprintf(macAddressTemplate, digits...)
}
