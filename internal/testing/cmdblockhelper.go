// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func AssertOperationWasBlocked(c *gc.C, err error, msg string) {
	c.Assert(err.Error(), jc.Contains, "disabled", gc.Commentf("%s", errors.Details(err)))
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, msg)
	c.Check(stripped, jc.Contains, "disabled")
}
