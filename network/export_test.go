// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

func SetPreferIPv6(value bool) {
	preferIPv6 = value
}

func GetPreferIPv6() bool {
	return preferIPv6
}
