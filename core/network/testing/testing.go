// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

// GenLinkLayerDeviceUUID can be used in testing for generating a link layer
// device uuid that is checked for subsequent errors using the test suites
// tc instance.
func GenLinkLayerDeviceUUID(c *tc.C) network.LinkLayerDeviceUUID {
	uuid, err := network.NewLinkLayerDeviceUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

// GenNetNodeUUID can be used in testing for generating a net node uuid
// that is checked for subsequent errors using the test suites tc instance.
func GenNetNodeUUID(c *tc.C) network.NetNodeUUID {
	uuid, err := network.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}
