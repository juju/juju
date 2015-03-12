// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type contentAsserterSuite struct{}

var _ = gc.Suite(&contentAsserterSuite{})

func (s *contentAsserterSuite) TestContentAsserterC(c *gc.C) {
	ch := make(chan string, 1)
	contentC := testing.ContentAsserterC{C:c, Chan:ch}
	contentC.AssertNoReceive()
	close(ch)
	contentC.AssertClosed()
}

