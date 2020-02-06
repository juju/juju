// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/multiwatcher"
)

var _ = gc.Suite(&stoppedSuite{})

type stoppedSuite struct {
	testing.IsolationSuite
}

func (*stoppedSuite) TestIsErrStopped(c *gc.C) {
	c.Assert(multiwatcher.NewErrStopped(), jc.Satisfies, multiwatcher.IsErrStopped)
	err := multiwatcher.ErrStoppedf("something")
	c.Assert(err, jc.Satisfies, multiwatcher.IsErrStopped)
	c.Assert(err.Error(), gc.Equals, "something was stopped")
}
