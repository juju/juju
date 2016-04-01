// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type maas2InstanceSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&maas2InstanceSuite{})

func (s *maas2InstanceSuite) TestString(c *gc.C) {
	instance := maas2Instance{fakeMachine{}}
}
