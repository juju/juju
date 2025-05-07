// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coreapplication "github.com/juju/juju/core/application"
)

// GenApplicationUUID can be used in testing for generating a application id
// that is checked for subsequent errors using the test suits go check instance.
func GenApplicationUUID(c *tc.C) coreapplication.ID {
	uuid, err := coreapplication.NewID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
