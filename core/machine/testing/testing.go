// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coremachine "github.com/juju/juju/core/machine"
)

// GenUUID can be used in testing for generating a machine uuid that is
// checked for errors using the test suit's go check instance.
func GenUUID(c *tc.C) coremachine.UUID {
	uuid, err := coremachine.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
