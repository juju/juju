// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container/kvm/mock"
	"launchpad.net/juju-core/testing/testbase"
)

type MockSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&MockSuite{})

func (*MockSuite) TestListInitiallyEmpty(c *gc.C) {
	factory := mock.MockFactory()
	containers, err := factory.List()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 0)
}
