// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container/kvm"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type KVMSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&KVMSuite{})

func (*KVMSuite) TestIsKVMSupported(c *gc.C) {
	c.Check(kvm.IsKVMSupported(), jc.IsTrue)
	c.Fail()
}
