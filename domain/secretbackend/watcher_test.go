// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	gc "gopkg.in/check.v1"

	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchWithAdd(c *gc.C) {

}

func (s *watcherSuite) TestWatchWithDelete(c *gc.C) {

}
