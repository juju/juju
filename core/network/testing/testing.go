// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

// GenApplicationUUID can be used in testing for generating a application id
// that is checked for subsequent errors using the test suits go check instance.
func GenSpaceUUID(c *tc.C) network.SpaceUUID {
	uuid, err := network.NewSpaceUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}
