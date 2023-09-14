// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"fmt"
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ControllersSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store          jujuclient.ClientStore
	controllerName string
	controller     jujuclient.ControllerDetails
}

var _ = gc.Suite(&ControllersSuite{})

func (s *ControllersSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	s.controllerName = "test.controller"
	s.controller = jujuclient.ControllerDetails{
		ControllerUUID: "test.uuid",
		APIEndpoints:   []string{"test.api.endpoint"},
		CACert:         "test.ca.cert",
		Cloud:          "aws",
		CloudRegion:    "southeastasia",
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
	c.Assert(found, jc.DeepEquals, &expected)
}

func (s *ControllersSuite) TestControllerByAPIEndpoints(c *gc.C) {
	name := firstTestControllerName(c)
	expected := s.getControllers(c)[name]
	found, foundName, err := s.store.ControllerByAPIEndpoints(expected.APIEndpoints...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, &expected)
	c.Assert(foundName, gc.Equals, name)
}

func (s *ControllersSuite) TestControllerByAPIEndpointsNoneExists(c *gc.C) {
	writeTestControllersFile(c)
	found, foundName, err := s.store.ControllerByAPIEndpoints("1.1.1.1:17070")
	c.Assert(err, gc.ErrorMatches, "controller with API endpoints .* not found")
	c.Assert(found, gc.IsNil)
	c.Assert(foundName, gc.Equals, "")
}

func (s *ControllersSuite) TestAddController(c *gc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *ControllersSuite) TestAddControllerDupUUIDFails(c *gc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
	// Try to add it again
	err = s.store.AddController(s.controllerName+"-copy", s.controller)
	c.Assert(err, gc.ErrorMatches, `controller with UUID .* already exists`)
}

func (s *ControllersSuite) TestAddControllerDupNameFails(c *gc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
	// Try to add it again
	err = s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, gc.ErrorMatches, `controller with name .* already exists`)
}

func (s *ControllersSuite) TestUpdateControllerAddFirst(c *gc.C) {
	// UpdateController should fail if no controller has first been added
	// with AddController.
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, gc.ErrorMatches, `controllers not found`)
}

func (s *ControllersSuite) TestUpdateControllerAddNew(c *gc.C) {
	// UpdateController should fail if no controller has first been added
	// with AddController.
	s.assertControllerNotExists(c)
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, gc.ErrorMatches, `controller .*not found`)
}

func (s *ControllersSuite) TestUpdateController(c *gc.C) {
	s.controllerName = firstTestControllerName(c)
	all := writeTestControllersFile(c)
	// This is not a restore (backup), so update with the existing UUID.
	s.controller.ControllerUUID = all.Controllers[s.controllerName].ControllerUUID
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

// Try and fail to use an existing controller's UUID to update another exisiting
// controller's config.
func (s *ControllersSuite) TestUpdateControllerDupUUID(c *gc.C) {
	firstControllerName := firstTestControllerName(c)
	all := writeTestControllersFile(c)
	firstControllerUUID := all.Controllers[firstControllerName].ControllerUUID
	for name, details := range all.Controllers {
		if details.ControllerUUID != firstControllerUUID {
			details.ControllerUUID = firstControllerUUID
			err := s.store.UpdateController(name, details)
			c.Assert(err, gc.ErrorMatches, `controller .* with UUID .* already exists`)
		}
	}
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

func (s *ControllersSuite) TestCurrentControllerNoneExists(c *gc.C) {
	_, err := s.store.CurrentController()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, "current controller not found")
}

func (s *ControllersSuite) TestRemoveControllerRemovesCookieJar(c *gc.C) {
	name := firstTestControllerName(c)

	jar, err := s.store.CookieJar(name)
	c.Assert(err, jc.ErrorIsNil)
	err = jar.Save()
	c.Assert(err, jc.ErrorIsNil)

	// Sanity-check that the cookie jar file exists.
	_, err = os.Stat(jujuclient.JujuCookiePath(name))
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.RemoveController(name)
	c.Assert(err, jc.ErrorIsNil)

	found, err := s.store.ControllerByName(name)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("controller %v not found", name))
	c.Assert(found, gc.IsNil)

	// Check that the cookie jar has been removed.
	_, err = os.Stat(jujuclient.JujuCookiePath(name))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *ControllersSuite) TestCurrentController(c *gc.C) {
	writeTestControllersFile(c)

	current, err := s.store.CurrentController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, "mallards")
}

func (s *ControllersSuite) TestSetCurrentController(c *gc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentController(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)

	controllers, err := jujuclient.ReadControllersFile(jujuclient.JujuControllersPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers.CurrentController, gc.Equals, s.controllerName)
}

func (s *ControllersSuite) TestSetCurrentControllerNoneExists(c *gc.C) {
	err := s.store.SetCurrentController(s.controllerName)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, "controller test.controller not found")
}

func (s *ControllersSuite) assertControllerNotExists(c *gc.C) {
	all := writeTestControllersFile(c)
	_, exists := all.Controllers[s.controllerName]
	c.Assert(exists, jc.IsFalse)
}

func (s *ControllersSuite) assertUpdateSucceeded(c *gc.C) {
	ctl := s.getControllers(c)[s.controllerName]
	ctl.DNSCache = nil
	c.Assert(ctl, jc.DeepEquals, s.controller)
}

func (s *ControllersSuite) getControllers(c *gc.C) map[string]jujuclient.ControllerDetails {
	controllers, err := s.store.AllControllers()
	c.Assert(err, jc.ErrorIsNil)
	return controllers
}

func firstTestControllerName(c *gc.C) string {
	all := writeTestControllersFile(c)
	for key := range all.Controllers {
		return key
	}
	return ""
}
