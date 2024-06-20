// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
)

// GenCharmUUID can be used in testing for generating a charm uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenCharmUUID(c *gc.C) corecharm.UUID {
	uuid, err := corecharm.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
