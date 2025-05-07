// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type SetModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store          jujuclient.ClientStore
	controllerName string
	controller     jujuclient.ControllerDetails
}

var _ = tc.Suite(&SetModelsSuite{})

func (s *SetModelsSuite) SetUpTest(c *tc.C) {
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

	err := s.store.AddController(s.controllerName, s.controller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.AllModels(s.controllerName)
	c.Assert(err, tc.ErrorMatches, "models for controller test.controller not found")
}

func (s *SetModelsSuite) TearDownTest(c *tc.C) {
	s.controller = jujuclient.ControllerDetails{}
	s.controllerName = ""
	s.store = nil
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *SetModelsSuite) TestSetModelsUnknownController(c *tc.C) {
	err := s.store.SetModels("some-kontroller-not-in-store", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SetModelsSuite) TestSetModelsNoControllerModels(c *tc.C) {
	err := s.store.SetModels(s.controllerName, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.AllModels(s.controllerName)
	c.Assert(err, tc.ErrorMatches, "models for controller test.controller not found")
}

func (s *SetModelsSuite) TestSetModelsAddOne(c *tc.C) {
	modelDetails := s.assertUpdateModel(c, "admin/new-model", "test.model.uuid")
	expected := map[string]jujuclient.ModelDetails{"admin/new-model": modelDetails}
	s.assertSetModels(c, expected)
}

func (s *SetModelsSuite) TestSetModelsAddMany(c *tc.C) {
	expected := map[string]jujuclient.ModelDetails{
		"admin/new-model":     s.assertUpdateModel(c, "admin/new-model", "test.model.uuid"),
		"admin/another-model": s.assertUpdateModel(c, "admin/another-model", "test.model.uuid.2"),
	}
	s.assertSetModels(c, expected)
}

func (s *SetModelsSuite) TestControllerModelsUpdate(c *tc.C) {
	expected := map[string]jujuclient.ModelDetails{
		"admin/new-model":     s.assertUpdateModel(c, "admin/new-model", "test.model.uuid"),
		"admin/another-model": s.assertUpdateModel(c, "admin/another-model", "test.model.uuid.2"),
	}
	s.assertSetModels(c, expected)
	// Make another call to ensure that we still have the same models information.
	s.assertSetModels(c, expected)
}

func (s *SetModelsSuite) TestSetModelsDeleteOne(c *tc.C) {
	detailsToLeave := s.assertUpdateModel(c, "admin/new-model", "test.model.uuid")
	before := map[string]jujuclient.ModelDetails{
		"admin/new-model":     detailsToLeave,
		"admin/another-model": s.assertUpdateModel(c, "admin/another-model", "test.model.uuid.2"),
	}
	after := map[string]jujuclient.ModelDetails{
		"admin/new-model": detailsToLeave,
	}

	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, tc.DeepEquals, before)
	s.assertSetModels(c, after)
}

func (s *SetModelsSuite) TestSetModelsDeleteAll(c *tc.C) {
	before := map[string]jujuclient.ModelDetails{
		"admin/new-model":     s.assertUpdateModel(c, "admin/new-model", "test.model.uuid"),
		"admin/another-model": s.assertUpdateModel(c, "admin/another-model", "test.model.uuid.2"),
	}
	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, tc.DeepEquals, before)
	s.assertSetModels(c, nil)
}

func (s *SetModelsSuite) TestSetModelsAddUpdateDeleteCombination(c *tc.C) {
	detailsToUpdate := s.assertUpdateModel(c, "admin/update-model", "test.model.uuid.2")
	before := map[string]jujuclient.ModelDetails{
		"admin/delete-model": s.assertUpdateModel(c, "admin/delete-model", "test.model.uuid"),
		"admin/update-model": detailsToUpdate,
	}
	after := map[string]jujuclient.ModelDetails{
		"admin/new-model": {
			ModelUUID: "test.model.uuid",
			ModelType: model.IAAS,
		},
		"admin/update-model": detailsToUpdate,
	}

	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, tc.DeepEquals, before)
	s.assertSetModels(c, after)
}

func (s *SetModelsSuite) TestSetModelsDoesNotUpdateCurrentModel(c *tc.C) {
	before := map[string]jujuclient.ModelDetails{
		"admin/delete-model": s.assertUpdateModel(c, "admin/delete-model", "test.model.uuid"),
	}
	s.store.SetCurrentModel(s.controllerName, "admin/delete-model")

	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, tc.DeepEquals, before)
	s.assertSetModels(c, nil)
	currentModel, err := s.store.CurrentModel(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentModel, tc.DeepEquals, "admin/delete-model")
}

func (s *SetModelsSuite) TestSetModelsControllerIsolated(c *tc.C) {
	before := map[string]jujuclient.ModelDetails{
		"admin/new-model": s.assertUpdateModel(c, "admin/new-model", "test.model.uuid"),
	}

	s.controller.ControllerUUID = "another.test.kontroller.uuid"
	err := s.store.AddController("another-kontroller", s.controller)
	c.Assert(err, jc.ErrorIsNil)
	otherModels := map[string]jujuclient.ModelDetails{
		"admin/foreign-model": {
			ModelUUID: "test.foreign.model.uuid",
			ModelType: model.IAAS,
		},
	}
	err = s.store.SetModels("another-kontroller", otherModels)
	c.Assert(err, jc.ErrorIsNil)

	// initial controller un-touched
	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, tc.DeepEquals, before)
}

func (s *SetModelsSuite) assertSetModels(c *tc.C, models map[string]jujuclient.ModelDetails) {
	err := s.store.SetModels(s.controllerName, models)
	c.Assert(err, jc.ErrorIsNil)
	storedModels, err := s.store.AllModels(s.controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedModels, tc.DeepEquals, models)
}

func (s *SetModelsSuite) assertUpdateModel(c *tc.C, modelName, modelUUID string) jujuclient.ModelDetails {
	modelDetails := jujuclient.ModelDetails{
		ModelUUID: modelUUID,
		ModelType: model.IAAS,
	}
	err := s.store.UpdateModel(s.controllerName, modelName, modelDetails)
	c.Assert(err, jc.ErrorIsNil)
	return modelDetails
}
