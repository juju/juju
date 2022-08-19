// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type containerFactorySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&containerFactorySuite{})

func (containerFactorySuite) TestNewContainerStartedIsNil(c *gc.C) {
	vm := new(containerFactory).New("some-kvm")

	raw, ok := vm.(*kvmContainer)
	c.Assert(ok, jc.IsTrue)

	// A new container instantiated in this way must have an "unknown"
	// started state, which will get queried and set at need.
	c.Assert(raw.started, gc.IsNil)
}
