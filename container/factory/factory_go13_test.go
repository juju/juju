// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

// +build go1.3

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
)

func (*factorySuite) TestNewContainerManagerLXD(c *gc.C) {
	testContainerManager(c, containerTest{
		containerType: instance.LXD,
		valid:         true,
	})
}
