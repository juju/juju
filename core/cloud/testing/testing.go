// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecloud "github.com/juju/juju/core/cloud"
)

// GenCloudUUID can be used in testing for generating a cloud uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenCloudUUID(c *gc.C) corecloud.UUID {
	uuid, err := corecloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
