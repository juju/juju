// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"net"

	"github.com/juju/tc"
)

type ipsEqualChecker struct {
	*tc.CheckerInfo
}

var IPsEqual tc.Checker = &ipsEqualChecker{
	&tc.CheckerInfo{Name: "IPsEqual", Params: []string{"obtained", "expected"}},
}

func (c *ipsEqualChecker) Check(params []interface{}, name []string) (bool, string) {
	ips1, ok := params[0].([]net.IP)
	if !ok {
		return false, "param 0 is not of type []net.IP"
	}
	ips2, ok := params[1].([]net.IP)
	if !ok {
		return false, "param 0 is not of type []net.IP"
	}

	if len(ips1) != len(ips2) {
		return false, fmt.Sprintf("legnth of ip slices not equal %d != %d",
			len(ips1), len(ips2))
	}

	for i := range ips1 {
		if !ips1[i].Equal(ips2[i]) {
			return false, "ip slices are not equal"
		}
	}
	return true, ""
}
