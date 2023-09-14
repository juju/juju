// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store jujuclient.ModelStore
}

var _ = gc.Suite(&ModelsSuite{})

func (s *ModelsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	writeTestModelsFile(c)
}

func (s *ModelsSuite) TestModelByNameNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	details, err := s.store.ModelByName("not-found", "admin/admin")
	c.Assert(err, gc.ErrorMatches, "model not-found:admin/admin not found")
	c.Assert(details, gc.IsNil)
}

func (s *ModelsSuite) TestModelByNameControllerNotFound(c *gc.C) {
	details, err := s.store.ModelByName("not-found", "admin/admin")
	c.Assert(err, gc.ErrorMatches, "model not-found:admin/admin not found")
	c.Assert(details, gc.IsNil)
}

func (s *ModelsSuite) TestModelByNameModelNotFound(c *gc.C) {
	details, err := s.store.ModelByName("kontroll", "admin/not-found")
	c.Assert(err, gc.ErrorMatches, "model kontroll:admin/not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *ModelsSuite) TestModelByName(c *gc.C) {
	details, err := s.store.ModelByName("kontroll", "admin/admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.NotNil)
	c.Assert(*details, jc.DeepEquals, testControllerModels["kontroll"].Models["admin/admin"])
}

func (s *ModelsSuite) TestAllModelsNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	models, err := s.store.AllModels("not-found")
	c.Assert(err, gc.ErrorMatches, "models for controller not-found not found")
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelsSuite) TestAllModels(c *gc.C) {
	models, err := s.store.AllModels("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, testControllerModels["kontroll"].Models)
}

func (s *ModelsSuite) TestCurrentModel(c *gc.C) {
	current, err := s.store.CurrentModel("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, "admin/my-model")
}

func (s *ModelsSuite) TestCurrentModelNotSet(c *gc.C) {
	_, err := s.store.CurrentModel("ctrl")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestCurrentModelControllerNotFound(c *gc.C) {
	_, err := s.store.CurrentModel("not-found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestSetCurrentModelControllerNotFound(c *gc.C) {
	err := s.store.SetCurrentModel("not-found", "admin/admin")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestSetCurrentModelModelNotFound(c *gc.C) {
	err := s.store.SetCurrentModel("kontroll", "admin/not-found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestSetCurrentModel(c *gc.C) {
	err := s.store.SetCurrentModel("kontroll", "admin/admin")
	c.Assert(err, jc.ErrorIsNil)
	all, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all["kontroll"].CurrentModel, gc.Equals, "admin/admin")
}

func (s *ModelsSuite) TestUpdateModelNewController(c *gc.C) {
	testModelDetails := jujuclient.ModelDetails{
		ModelUUID:    "test.uuid",
		ModelType:    model.IAAS,
		ActiveBranch: model.GenerationMaster,
	}
	err := s.store.UpdateModel("new-controller", "admin/new-model", testModelDetails)
	c.Assert(err, jc.ErrorIsNil)
	models, err := s.store.AllModels("new-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/new-model": testModelDetails,
	})
}

func (s *ModelsSuite) TestUpdateModelExistingControllerAndModelNewModel(c *gc.C) {
	testModelDetails := jujuclient.ModelDetails{
		ModelUUID:    "test.uuid",
		ModelType:    model.IAAS,
		ActiveBranch: model.GenerationMaster,
	}
	err := s.store.UpdateModel("kontroll", "admin/new-model", testModelDetails)
	c.Assert(err, jc.ErrorIsNil)
	models, err := s.store.AllModels("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/admin":     kontrollAdminModelDetails,
		"admin/my-model":  kontrollMyModelModelDetails,
		"admin/new-model": testModelDetails,
	})
}

func (s *ModelsSuite) TestUpdateModelOverwrites(c *gc.C) {
	testModelDetails := jujuclient.ModelDetails{
		ModelUUID:    "test.uuid",
		ModelType:    model.IAAS,
		ActiveBranch: model.GenerationMaster,
	}
	for i := 0; i < 2; i++ {
		// Twice so we exercise the code path of updating with
		// identical details.
		err := s.store.UpdateModel("kontroll", "admin/admin", testModelDetails)
		c.Assert(err, jc.ErrorIsNil)
		details, err := s.store.ModelByName("kontroll", "admin/admin")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*details, jc.DeepEquals, testModelDetails)
	}
}

func (s *ModelsSuite) TestUpdateModelRejectsTypeChange(c *gc.C) {
	testModelDetails := jujuclient.ModelDetails{ModelUUID: "test.uuid", ModelType: model.CAAS}
	err := s.store.UpdateModel("kontroll", "admin/my-model", testModelDetails)
	c.Assert(err, gc.ErrorMatches, `model type was "iaas", cannot change to "caas"`)
}

func (s *ModelsSuite) TestUpdateModelEmptyModels(c *gc.C) {
	// This test exists to exercise a bug caused by the
	// presence of a file with an empty "models" field,
	// that would lead to a panic.
	err := os.WriteFile(jujuclient.JujuModelsPath(), []byte(`
controllers:
  ctrl:
    models:
`[1:]), 0644)
	c.Assert(err, jc.ErrorIsNil)

	testModelDetails := jujuclient.ModelDetails{
		ModelUUID:    "test.uuid",
		ModelType:    model.IAAS,
		ActiveBranch: model.GenerationMaster,
	}
	err = s.store.UpdateModel("ctrl", "admin/admin", testModelDetails)
	c.Assert(err, jc.ErrorIsNil)
	models, err := s.store.AllModels("ctrl")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/admin": testModelDetails,
	})
}

func (s *ModelsSuite) TestRemoveModelNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.RemoveModel("not-found", "admin/admin")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveModelControllerNotFound(c *gc.C) {
	err := s.store.RemoveModel("not-found", "admin/admin")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveModelNotFound(c *gc.C) {
	err := s.store.RemoveModel("kontroll", "admin/not-found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveModel(c *gc.C) {
	err := s.store.RemoveModel("kontroll", "admin/admin")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.ModelByName("kontroll", "admin/admin")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveControllerRemovesModels(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	err := store.AddController("kontroll", jujuclient.ControllerDetails{
		ControllerUUID: "abc",
		CACert:         "woop",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = store.RemoveController("kontroll")
	c.Assert(err, jc.ErrorIsNil)

	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	_, ok := models["admin/kontroll"]
	c.Assert(ok, jc.IsFalse) // kontroll models are removed
}
