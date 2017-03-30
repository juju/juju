// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"

	"github.com/pkg/errors"
)

// getMacAndIp picks and returns the correct mac and ip from the given slice
// if the slice does not contain a valid mac and ip it will return an error
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
		_, err = net.ParseMAC(val)
		if err != nil {
			err = errors.Errorf("The address is not an mac neighter an ip %s", val)
			break
		}
		mac = val
	}
	return
}
