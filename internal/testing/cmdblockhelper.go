// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
)

func AssertOperationWasBlocked(c *tc.C, err error, msg string) {
	c.Assert(err.Error(), tc.Contains, "disabled", tc.Commentf("%s", errors.Details(err)))
}
