// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

func AssertOperationWasBlocked(c *tc.C, err error, msg string) {
	c.Assert(err.Error(), jc.Contains, "disabled", tc.Commentf("%s", errors.Details(err)))
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, tc.Matches, msg)
	c.Check(stripped, jc.Contains, "disabled")
}
