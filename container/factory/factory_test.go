// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container"
	"github.com/juju/juju/container/factory"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/testing"
)

type factorySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&factorySuite{})

func (*factorySuite) TestNewContainerManager(c *gc.C) {
	for _, test := range []struct {
		containerType instance.ContainerType
		valid         bool
	}{{
		containerType: instance.LXC,
		valid:         true,
	}, {
		containerType: instance.KVM,
		valid:         true,
	}, {
		containerType: instance.NONE,
		valid:         false,
	}, {
		containerType: instance.ContainerType("other"),
		valid:         false,
	}} {
		conf := container.ManagerConfig{container.ConfigName: "test"}
		manager, err := factory.NewContainerManager(test.containerType, conf, nil)
		if test.valid {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(manager, gc.NotNil)
		} else {
			c.Assert(err, gc.ErrorMatches, `unknown container type: ".*"`)
			c.Assert(manager, gc.IsNil)
		}
	}
}
