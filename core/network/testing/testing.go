// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

// GenLinkLayerDeviceUUID can be used in testing for generating a link layer
// device uuid that is checked for subsequent errors using the test suits go
// check instance.
func GenLinkLayerDeviceUUID(c *tc.C) network.LinkLayerDeviceUUID {
	uuid, err := network.NewLinkLayerDeviceUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}
