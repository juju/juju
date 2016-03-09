// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ControllersSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store          jujuclient.ControllerStore
	controllerName string
	controller     jujuclient.ControllerDetails
}

var _ = gc.Suite(&ControllersSuite{})

func (s *ControllersSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	s.controllerName = "test.controller"
	s.controller = jujuclient.ControllerDetails{
		[]string{"test.server.hostname"},
		"test.uuid",
		[]string{"test.api.endpoint"},
		"test.ca.cert",
	}
}

func (s *ControllersSuite) TestControllerMetadataNone(c *gc.C) {
	c.Assert(s.getControllers(c), gc.IsNil)
}

func (s *ControllersSuite) TestControllerByNameNoFile(c *gc.C) {
	found, err := s.store.ControllerByName(s.controllerName)
	c.Assert(err, gc.ErrorMatches, "controller test.controller not found")
	c.Assert(found, gc.IsNil)
}

func (s *ControllersSuite) TestControllerByNameNoneExists(c *gc.C) {
	writeTestControllersFile(c)
	found, err := s.store.ControllerByName(s.controllerName)
	c.Assert(err, gc.ErrorMatches, "controller test.controller not found")
	c.Assert(found, gc.IsNil)
}

func (s *ControllersSuite) TestControllerByName(c *gc.C) {
	name := firstTestControllerName(c)
	found, err := s.store.ControllerByName(name)
	c.Assert(err, jc.ErrorIsNil)
	expected := s.getControllers(c)[name]
	c.Assert(found, gc.DeepEquals, &expected)
}

func (s *ControllersSuite) TestUpdateControllerAddFirst(c *gc.C) {
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *ControllersSuite) TestUpdateControllerAddNew(c *gc.C) {
	s.assertControllerNotExists(c)
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *ControllersSuite) TestUpdateController(c *gc.C) {
	s.controllerName = firstTestControllerName(c)

	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *ControllersSuite) TestRemoveControllerNoFile(c *gc.C) {
	err := s.store.RemoveController(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllersSuite) TestRemoveControllerUnknown(c *gc.C) {
	s.assertControllerNotExists(c)
	err := s.store.RemoveController(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllersSuite) TestRemoveController(c *gc.C) {
	name := firstTestControllerName(c)

	err := s.store.RemoveController(name)
	c.Assert(err, jc.ErrorIsNil)

	found, err := s.store.ControllerByName(name)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("controller %v not found", name))
	c.Assert(found, gc.IsNil)
}

func (s *ControllersSuite) assertWriteFails(c *gc.C, failureMessage string) {
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, gc.ErrorMatches, failureMessage)

	found, err := s.store.ControllerByName(s.controllerName)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("controller %v not found", s.controllerName))
	c.Assert(found, gc.IsNil)
}

func (s *ControllersSuite) assertControllerNotExists(c *gc.C) {
	all := writeTestControllersFile(c)
	_, exists := all[s.controllerName]
	c.Assert(exists, jc.IsFalse)
}

func (s *ControllersSuite) assertUpdateSucceeded(c *gc.C) {
	c.Assert(s.getControllers(c)[s.controllerName], gc.DeepEquals, s.controller)
}

func (s *ControllersSuite) getControllers(c *gc.C) map[string]jujuclient.ControllerDetails {
	controllers, err := s.store.AllControllers()
	c.Assert(err, jc.ErrorIsNil)
	return controllers
}

func firstTestControllerName(c *gc.C) string {
	all := writeTestControllersFile(c)
	for key, _ := range all {
		return key
	}
	return ""
}
