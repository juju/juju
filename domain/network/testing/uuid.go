// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/domain/network"
)

// GenNetNodeUUID is a convenience testing function for generating a net node
// uuid during tests.
func GenNetNodeUUID(c *tc.C) network.NetNodeUUID {
	uuid, err := network.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}
