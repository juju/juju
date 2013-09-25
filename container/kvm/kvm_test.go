// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container/kvm"
	//jc "launchpad.net/juju-core/testing/checkers"
	//"launchpad.net/juju-core/testing/testbase"
)

type KVMSuite struct {
	kvm.TestSuite
}

var _ = gc.Suite(&KVMSuite{})

// TODO: work out how to test the actual kvm implementations.

func (*KVMSuite) TestListInitiallyEmpty(c *gc.C) {
	manager, err := kvm.NewContainerManager("test")
	c.Assert(err, gc.IsNil)
	containers, err := manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 0)
}
