// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"
	"regexp"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/testing"
)

type InfoSuite struct {
	testing.FakeJujuHomeSuite

	controllerName string
	info           controller.ControllerInfo
}

var _ = gc.Suite(&InfoSuite{})

func (s *InfoSuite) TestWriteNoName(c *gc.C) {
	s.constructTestController()
	s.info.Name = ""
	s.assertWriteFails(c, "missing name, controller info not valid")
}

func (s *InfoSuite) TestWriteNoControllerUUID(c *gc.C) {
	s.constructTestController()
	s.info.ControllerUUID = ""
	s.assertWriteFails(c, "missing uuid, controller info not valid")
}

func (s *InfoSuite) TestWriteNoCACert(c *gc.C) {
	s.constructTestController()
	s.info.CACert = ""
	s.assertWriteFails(c, "missing ca-cert, controller info not valid")
}

func (s *InfoSuite) TestWriteNoServers(c *gc.C) {
	s.constructTestController()
	s.info.Controller.Servers = []string{}
	s.assertWriteFails(c, regexp.QuoteMeta("missing server host name(s), controller info not valid"))
}

func (s *InfoSuite) TestWriteNoEndpoints(c *gc.C) {
	s.constructTestController()
	s.info.APIEndpoints = []string{}
	s.assertWriteFails(c, regexp.QuoteMeta("missing api endpoint(s), controller info not valid"))
}

func (s *InfoSuite) constructTestController() {
	s.controllerName = "test.controller.info"
	s.info = controller.ControllerInfo{
		controller.Controller{
			[]string{"host.names"},
			"controller.uuid",
			[]string{"api.endpoints"},
			"ca.cert",
		},
		s.controllerName,
	}
}

func (s *InfoSuite) assertWriteFails(c *gc.C, failureMessage string) {
	err := s.info.Write()
	c.Assert(err, gc.ErrorMatches, failureMessage)

	found, err := controller.ControllerByName(s.controllerName)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("controller %v not found", s.controllerName))
	c.Assert(found, gc.IsNil)
}
