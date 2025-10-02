// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
)

// GenRemoteApplicationUUID can be used in testing for generating a remote
// application uuid that is checked for subsequent errors using the test suits
// go check instance.
func GenRemoteApplicationUUID(c *tc.C) coreremoteapplication.UUID {
	uuid, err := coreremoteapplication.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}
