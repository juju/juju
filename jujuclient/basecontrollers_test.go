// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type baseControllersSuite struct {
	testing.FakeJujuXDGDataHomeSuite

	controllerName string
	controller     jujuclient.ControllerDetails
}

func (s *baseControllersSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.controllerName = "test.controller"
	s.controller = jujuclient.ControllerDetails{
		[]string{"test.server.hostname"},
		"test.uuid",
		[]string{"test.api.endpoint"},
		"test.ca.cert",
	}
}
