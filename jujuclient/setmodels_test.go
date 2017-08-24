// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ControllerModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store          jujuclient.ClientStore
	controllerName string
	controller     jujuclient.ControllerDetails
}

var _ = gc.Suite(&ControllerModelsSuite{})

func (s *ControllerModelsSuite) SetUpTest(c *gc.C) {
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

func (s *ControllerModelsSuite) TearDownTest(c *gc.C) {
	s.controller = jujuclient.ControllerDetails{}
	s.controllerName = ""
	s.store = nil
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *ControllerModelsSuite) TestSetModelsNoController(c *gc.C) {
	err := s.store.SetModels(s.controllerName, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerModelsSuite) TestSetModelsNoControllerModels(c *gc.C) {
	s.assertControllerStored(c)
	err := s.store.SetModels(s.controllerName, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.AllModels(s.controllerName)
	c.Assert(err, gc.ErrorMatches, "models for controller test.controller not found")
}

func (s *ControllerModelsSuite) TestSetModelsAddOne(c *gc.C) {
	s.assertControllerStored(c)
	modelDetails := s.assertModelStored(c, "admin/new-model", "test.model.uuid")
	expected := map[string]jujuclient.ModelDetails{"admin/new-model": modelDetails}
	s.assertStoreUpdated(c, expected)
}

func (s *ControllerModelsSuite) TestSetModelsAddMany(c *gc.C) {
	s.assertControllerStored(c)
	expected := map[string]jujuclient.ModelDetails{
		"admin/new-model":     s.assertModelStored(c, "admin/new-model", "test.model.uuid"),
		"admin/another-model": s.assertModelStored(c, "admin/another-model", "test.model.uuid.2"),
	}
	s.assertStoreUpdated(c, expected)
}

func (s *ControllerModelsSuite) TestControllerModelsUpdate(c *gc.C) {
	s.assertControllerStored(c)
	expected := map[string]jujuclient.ModelDetails{
		"admin/new-model":     s.assertModelStored(c, "admin/new-model", "test.model.uuid"),
		"admin/another-model": s.assertModelStored(c, "admin/another-model", "test.model.uuid.2"),
	}
	s.assertStoreUpdated(c, expected)
	s.assertStoreUpdated(c, expected)
}

func (s *ControllerModelsSuite) TestSetModelsDeleteOne(c *gc.C) {
	s.assertControllerStored(c)
	detailsToLeave := s.assertModelStored(c, "admin/new-model", "test.model.uuid")
	before := map[string]jujuclient.ModelDetails{
		"admin/new-model":     detailsToLeave,
		"admin/another-model": s.assertModelStored(c, "admin/another-model", "test.model.uuid.2"),
	}
	after := map[string]jujuclient.ModelDetails{
		"admin/new-model": detailsToLeave,
	}

	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, gc.DeepEquals, before)
	s.assertStoreUpdated(c, after)
}

func (s *ControllerModelsSuite) TestSetModelsDeleteAll(c *gc.C) {
	s.assertControllerStored(c)
	before := map[string]jujuclient.ModelDetails{
		"admin/new-model":     s.assertModelStored(c, "admin/new-model", "test.model.uuid"),
		"admin/another-model": s.assertModelStored(c, "admin/another-model", "test.model.uuid.2"),
	}
	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, gc.DeepEquals, before)
	s.assertStoreUpdated(c, nil)
}

func (s *ControllerModelsSuite) TestSetModelsAddUpdateDeleteCombination(c *gc.C) {
	s.assertControllerStored(c)
	detailsToUpdate := s.assertModelStored(c, "admin/update-model", "test.model.uuid.2")
	before := map[string]jujuclient.ModelDetails{
		"admin/delete-model": s.assertModelStored(c, "admin/delete-model", "test.model.uuid"),
		"admin/update-model": detailsToUpdate,
	}
	after := map[string]jujuclient.ModelDetails{
		"admin/new-model":    jujuclient.ModelDetails{"test.model.uuid"},
		"admin/update-model": detailsToUpdate,
	}

	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, gc.DeepEquals, before)
	s.assertStoreUpdated(c, after)
}

func (s *ControllerModelsSuite) TestSetModelsControllerIsolated(c *gc.C) {
	s.assertControllerStored(c)
	before := map[string]jujuclient.ModelDetails{
		"admin/new-model": s.assertModelStored(c, "admin/new-model", "test.model.uuid"),
	}

	s.controller.ControllerUUID = "another.test.kontroller.uuid"
	err := s.store.AddController("another-kontroller", s.controller)
	c.Assert(err, jc.ErrorIsNil)
	otherModels := map[string]jujuclient.ModelDetails{
		"admin/foreign-model": jujuclient.ModelDetails{"test.foreign.model.uuid"},
	}
	err = s.store.SetModels("another-kontroller", otherModels)
	c.Assert(err, jc.ErrorIsNil)

	// initial controller un-touched
	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, gc.DeepEquals, before)
}

func (s *ControllerModelsSuite) assertStoreUpdated(c *gc.C, models map[string]jujuclient.ModelDetails) {
	err := s.store.SetModels(s.controllerName, models)
	c.Assert(err, jc.ErrorIsNil)
	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, gc.DeepEquals, models)
}

func (s *ControllerModelsSuite) assertModelStored(c *gc.C, modelName, modelUUID string) jujuclient.ModelDetails {
	modelDetails := jujuclient.ModelDetails{modelUUID}
	err := s.store.UpdateModel(s.controllerName, modelName, modelDetails)
	c.Assert(err, jc.ErrorIsNil)
	return modelDetails
}

func (s *ControllerModelsSuite) assertControllerStored(c *gc.C) {
	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.AllModels(s.controllerName)
	c.Assert(err, gc.ErrorMatches, "models for controller test.controller not found")
}
