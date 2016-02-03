// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"fmt"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/testing"
)

type ControllersSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&ControllersSuite{})

func (s *ControllersSuite) TestControllerMetadataNone(c *gc.C) {
	c.Assert(getControllers(c), gc.IsNil)
}

func (s *ControllersSuite) TestControllerByNameNoFile(c *gc.C) {
	found, err := controller.ControllerByName(testControllerName)
	c.Assert(err, gc.ErrorMatches, "controller test.controller not found")
	c.Assert(found, gc.IsNil)
}

func (s *ControllersSuite) TestControllerByNameNoneExists(c *gc.C) {
	writeTestControllersFile(c)
	found, err := controller.ControllerByName(testControllerName)
	c.Assert(err, gc.ErrorMatches, "controller test.controller not found")
	c.Assert(found, gc.IsNil)
}

func (s *ControllersSuite) TestControllerByName(c *gc.C) {
	name := firstTestControllerName(c)
	found, err := controller.ControllerByName(name)
	c.Assert(err, jc.ErrorIsNil)
	expected := getControllers(c)[name]
	c.Assert(found, gc.DeepEquals, &expected)
}

func (s *ControllersSuite) TestUpdateControllerAddFirst(c *gc.C) {
	err := controller.UpdateController(testControllerName, testController)
	c.Assert(err, jc.ErrorIsNil)
	assertUpdateSucceeded(c, testControllerName)
}

func (s *ControllersSuite) TestUpdateControllerAddNew(c *gc.C) {
	assertControllerNotExists(c, testControllerName)
	err := controller.UpdateController(testControllerName, testController)
	c.Assert(err, jc.ErrorIsNil)
	assertUpdateSucceeded(c, testControllerName)
}

func (s *ControllersSuite) TestUpdateController(c *gc.C) {
	name := firstTestControllerName(c)

	err := controller.UpdateController(name, testController)
	c.Assert(err, jc.ErrorIsNil)
	assertUpdateSucceeded(c, name)
}

func (s *ControllersSuite) TestRemoveControllerNoFile(c *gc.C) {
	err := controller.RemoveController(testControllerName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllersSuite) TestRemoveControllerUnknown(c *gc.C) {
	assertControllerNotExists(c, testControllerName)
	err := controller.RemoveController(testControllerName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllersSuite) TestRemoveController(c *gc.C) {
	name := firstTestControllerName(c)

	err := controller.RemoveController(name)
	c.Assert(err, jc.ErrorIsNil)

	found, err := controller.ControllerByName(name)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("controller %v not found", name))
	c.Assert(found, gc.IsNil)
}

var (
	testControllerName = "test.controller"
	testController     = controller.Controller{
		[]string{"test.server.hostname"},
		"test.uuid",
		[]string{"test.api.endpoint"},
		"test.ca.cert",
	}
)

func assertUpdateSucceeded(c *gc.C, controllerName string) {
	c.Assert(getControllers(c)[controllerName], gc.DeepEquals, testController)
}

func getControllers(c *gc.C) map[string]controller.Controller {
	controllers, err := controller.ControllerMetadata()
	c.Assert(err, jc.ErrorIsNil)
	return controllers
}

func firstTestControllerName(c *gc.C) string {
	all := writeTestControllersFile(c)
	for key, _ := range all.Controllers {
		return key
	}
	return ""
}

func assertControllerNotExists(c *gc.C, name string) {
	all := writeTestControllersFile(c)
	_, exists := all.Controllers[name]
	c.Assert(exists, jc.IsFalse)
}
