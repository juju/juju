// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

var NetLookupIP = &netLookupIP

func SetPreferIPv6(value bool) {
	globalPreferIPv6 = value
}

func GetPreferIPv6() bool {
	return globalPreferIPv6
}
