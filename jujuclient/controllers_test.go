// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"fmt"
	"os"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ControllersSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store          jujuclient.ClientStore
	controllerName string
	controller     jujuclient.ControllerDetails
}

func TestControllersSuite(t *stdtesting.T) {
	tc.Run(t, &ControllersSuite{})
}

func (s *ControllersSuite) SetUpTest(c *tc.C) {
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

func (s *ControllersSuite) TestControllerMetadataNone(c *tc.C) {
	c.Assert(s.getControllers(c), tc.IsNil)
}

func (s *ControllersSuite) TestControllerByNameNoFile(c *tc.C) {
	found, err := s.store.ControllerByName(s.controllerName)
	c.Assert(err, tc.ErrorMatches, "controller test.controller not found")
	c.Assert(found, tc.IsNil)
}

func (s *ControllersSuite) TestControllerByNameNoneExists(c *tc.C) {
	writeTestControllersFile(c)
	found, err := s.store.ControllerByName(s.controllerName)
	c.Assert(err, tc.ErrorMatches, "controller test.controller not found")
	c.Assert(found, tc.IsNil)
}

func (s *ControllersSuite) TestControllerByName(c *tc.C) {
	name := firstTestControllerName(c)
	found, err := s.store.ControllerByName(name)
	c.Assert(err, tc.ErrorIsNil)
	expected := s.getControllers(c)[name]
	c.Assert(found, tc.DeepEquals, &expected)
}

func (s *ControllersSuite) TestControllerByAPIEndpoints(c *tc.C) {
	name := firstTestControllerName(c)
	expected := s.getControllers(c)[name]
	found, foundName, err := s.store.ControllerByAPIEndpoints(expected.APIEndpoints...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.DeepEquals, &expected)
	c.Assert(foundName, tc.Equals, name)
}

func (s *ControllersSuite) TestControllerByAPIEndpointsNoneExists(c *tc.C) {
	writeTestControllersFile(c)
	found, foundName, err := s.store.ControllerByAPIEndpoints("1.1.1.1:17070")
	c.Assert(err, tc.ErrorMatches, "controller with API endpoints .* not found")
	c.Assert(found, tc.IsNil)
	c.Assert(foundName, tc.Equals, "")
}

func (s *ControllersSuite) TestAddController(c *tc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

func (s *ControllersSuite) TestAddControllerDupUUIDFails(c *tc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
	// Try to add it again
	err = s.store.AddController(s.controllerName+"-copy", s.controller)
	c.Assert(err, tc.ErrorMatches, `controller with UUID .* already exists`)
}

func (s *ControllersSuite) TestAddControllerDupNameFails(c *tc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
	// Try to add it again
	err = s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorMatches, `controller with name .* already exists`)
}

func (s *ControllersSuite) TestUpdateControllerAddFirst(c *tc.C) {
	// UpdateController should fail if no controller has first been added
	// with AddController.
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorMatches, `controllers not found`)
}

func (s *ControllersSuite) TestUpdateControllerAddNew(c *tc.C) {
	// UpdateController should fail if no controller has first been added
	// with AddController.
	s.assertControllerNotExists(c)
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorMatches, `controller .*not found`)
}

func (s *ControllersSuite) TestUpdateController(c *tc.C) {
	s.controllerName = firstTestControllerName(c)
	all := writeTestControllersFile(c)
	// This is not a restore (backup), so update with the existing UUID.
	s.controller.ControllerUUID = all.Controllers[s.controllerName].ControllerUUID
	err := s.store.UpdateController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdateSucceeded(c)
}

// Try and fail to use an existing controller's UUID to update another exisiting
// controller's config.
func (s *ControllersSuite) TestUpdateControllerDupUUID(c *tc.C) {
	firstControllerName := firstTestControllerName(c)
	all := writeTestControllersFile(c)
	firstControllerUUID := all.Controllers[firstControllerName].ControllerUUID
	for name, details := range all.Controllers {
		if details.ControllerUUID != firstControllerUUID {
			details.ControllerUUID = firstControllerUUID
			err := s.store.UpdateController(name, details)
			c.Assert(err, tc.ErrorMatches, `controller .* with UUID .* already exists`)
		}
	}
}

func (s *ControllersSuite) TestRemoveControllerNoFile(c *tc.C) {
	err := s.store.RemoveController(s.controllerName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllersSuite) TestRemoveControllerUnknown(c *tc.C) {
	s.assertControllerNotExists(c)
	err := s.store.RemoveController(s.controllerName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllersSuite) TestRemoveController(c *tc.C) {
	name := firstTestControllerName(c)

	err := s.store.RemoveController(name)
	c.Assert(err, tc.ErrorIsNil)

	found, err := s.store.ControllerByName(name)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("controller %v not found", name))
	c.Assert(found, tc.IsNil)
}

func (s *ControllersSuite) TestCurrentControllerNoneExists(c *tc.C) {
	_, err := s.store.CurrentController()
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(err, tc.ErrorMatches, "current controller not found")
}

func (s *ControllersSuite) TestRemoveControllerRemovesCookieJar(c *tc.C) {
	name := firstTestControllerName(c)

	jar, err := s.store.CookieJar(name)
	c.Assert(err, tc.ErrorIsNil)
	err = jar.Save()
	c.Assert(err, tc.ErrorIsNil)

	// Sanity-check that the cookie jar file exists.
	_, err = os.Stat(jujuclient.JujuCookiePath(name))
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.RemoveController(name)
	c.Assert(err, tc.ErrorIsNil)

	found, err := s.store.ControllerByName(name)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("controller %v not found", name))
	c.Assert(found, tc.IsNil)

	// Check that the cookie jar has been removed.
	_, err = os.Stat(jujuclient.JujuCookiePath(name))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *ControllersSuite) TestCurrentController(c *tc.C) {
	writeTestControllersFile(c)

	current, err := s.store.CurrentController()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(current, tc.Equals, "mallards")
}

func (s *ControllersSuite) TestSetCurrentController(c *tc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, tc.ErrorIsNil)
	err = s.store.SetCurrentController(s.controllerName)
	c.Assert(err, tc.ErrorIsNil)

	controllers, err := jujuclient.ReadControllersFile(jujuclient.JujuControllersPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllers.CurrentController, tc.Equals, s.controllerName)
}

func (s *ControllersSuite) TestSetCurrentControllerNoneExists(c *tc.C) {
	err := s.store.SetCurrentController(s.controllerName)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(err, tc.ErrorMatches, "controller test.controller not found")
}

func (s *ControllersSuite) assertControllerNotExists(c *tc.C) {
	all := writeTestControllersFile(c)
	_, exists := all.Controllers[s.controllerName]
	c.Assert(exists, tc.IsFalse)
}

func (s *ControllersSuite) assertUpdateSucceeded(c *tc.C) {
	ctl := s.getControllers(c)[s.controllerName]
	ctl.DNSCache = nil
	c.Assert(ctl, tc.DeepEquals, s.controller)
}

func (s *ControllersSuite) getControllers(c *tc.C) map[string]jujuclient.ControllerDetails {
	controllers, err := s.store.AllControllers()
	c.Assert(err, tc.ErrorIsNil)
	return controllers
}

func firstTestControllerName(c *tc.C) string {
	all := writeTestControllersFile(c)
	for key := range all.Controllers {
		return key
	}
	return ""
}
