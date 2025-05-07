// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	corecloud "github.com/juju/juju/core/cloud"
)

// GenCloudUUID can be used in testing for generating a cloud uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenCloudUUID(c *tc.C) corecloud.UUID {
	uuid, err := corecloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
