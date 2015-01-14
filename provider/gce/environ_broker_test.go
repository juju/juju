// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type environBrokerSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environBrokerSuite{})

func (*environBrokerSuite) TestStartInstance(c *gc.C) {
}

func (*environBrokerSuite) TestFinishMachineConfig(c *gc.C) {
}

func (*environBrokerSuite) TestFindInstanceSpec(c *gc.C) {
}

func (*environBrokerSuite) TestNewRawInstance(c *gc.C) {
}

func (*environBrokerSuite) TestGetMetadata(c *gc.C) {
}

func (*environBrokerSuite) TestGetDisks(c *gc.C) {
}

func (*environBrokerSuite) TestGetHardwareCharacteristics(c *gc.C) {
}

func (*environBrokerSuite) TestAllInstances(c *gc.C) {
}

func (*environBrokerSuite) TestStopInstances(c *gc.C) {
}
