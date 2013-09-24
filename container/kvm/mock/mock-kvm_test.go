// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/container/kvm/mock"
	jc "launchpad.net/juju-core/testing/checkers"
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

func (*MockSuite) TestNewContainersInList(c *gc.C) {
	factory := mock.MockFactory()
	added := []kvm.Container{}
	added = append(added, factory.New("first"))
	added = append(added, factory.New("second"))
	containers, err := factory.List()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, jc.SameContents, added)
}
