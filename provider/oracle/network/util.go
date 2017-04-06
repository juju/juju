// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"

	"github.com/juju/errors"
)

// getMacAndIp is a helper function that returns a mac and an IP,
// given a list of strings containing both. This type of array
// is returned by the oracle API as part of instance details.
func getMacAndIP(address []string) (mac string, ip string, err error) {
	if address == nil {
		err = errors.New("Empty address slice given")
		return
	}
	for _, val := range address {
		valIp := net.ParseIP(val)
		if valIp != nil {
			ip = val
			continue
		}
		if _, err = net.ParseMAC(val); err != nil {
			err = errors.Errorf("The address is not an mac neither an ip %s", val)
			break
		}
		mac = val
	}
	return
}
