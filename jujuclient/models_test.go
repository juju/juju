// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store jujuclient.ModelStore
}

func TestModelsSuite(t *stdtesting.T) { tc.Run(t, &ModelsSuite{}) }
func (s *ModelsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	writeTestModelsFile(c)
}

func (s *ModelsSuite) TestModelByNameNoFile(c *tc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, tc.ErrorIsNil)
	details, err := s.store.ModelByName("not-found", "admin/admin")
	c.Assert(err, tc.ErrorMatches, "model not-found:admin/admin not found")
	c.Assert(details, tc.IsNil)
}

func (s *ModelsSuite) TestModelByNameControllerNotFound(c *tc.C) {
	details, err := s.store.ModelByName("not-found", "admin/admin")
	c.Assert(err, tc.ErrorMatches, "model not-found:admin/admin not found")
	c.Assert(details, tc.IsNil)
}

func (s *ModelsSuite) TestModelByNameModelNotFound(c *tc.C) {
	details, err := s.store.ModelByName("kontroll", "admin/not-found")
	c.Assert(err, tc.ErrorMatches, "model kontroll:admin/not-found not found")
	c.Assert(details, tc.IsNil)
}

func (s *ModelsSuite) TestModelByName(c *tc.C) {
	details, err := s.store.ModelByName("kontroll", "admin/admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details, tc.NotNil)
	c.Assert(*details, tc.DeepEquals, testControllerModels["kontroll"].Models["admin/admin"])
}

func (s *ModelsSuite) TestAllModelsNoFile(c *tc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, tc.ErrorIsNil)
	models, err := s.store.AllModels("not-found")
	c.Assert(err, tc.ErrorMatches, "models for controller not-found not found")
	c.Assert(models, tc.HasLen, 0)
}

func (s *ModelsSuite) TestAllModels(c *tc.C) {
	models, err := s.store.AllModels("kontroll")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, testControllerModels["kontroll"].Models)
}

func (s *ModelsSuite) TestCurrentModel(c *tc.C) {
	current, err := s.store.CurrentModel("kontroll")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(current, tc.Equals, "admin/my-model")
}

func (s *ModelsSuite) TestCurrentModelNotSet(c *tc.C) {
	_, err := s.store.CurrentModel("ctrl")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestCurrentModelControllerNotFound(c *tc.C) {
	_, err := s.store.CurrentModel("not-found")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestSetCurrentModelControllerNotFound(c *tc.C) {
	err := s.store.SetCurrentModel("not-found", "admin/admin")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestSetCurrentModelModelNotFound(c *tc.C) {
	err := s.store.SetCurrentModel("kontroll", "admin/not-found")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestSetCurrentModel(c *tc.C) {
	err := s.store.SetCurrentModel("kontroll", "admin/admin")
	c.Assert(err, tc.ErrorIsNil)
	all, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(all["kontroll"].CurrentModel, tc.Equals, "admin/admin")
}

func (s *ModelsSuite) TestUpdateModelNewController(c *tc.C) {
	testModelDetails := jujuclient.ModelDetails{
		ModelUUID: "test.uuid",
		ModelType: model.IAAS,
	}
	err := s.store.UpdateModel("new-controller", "admin/new-model", testModelDetails)
	c.Assert(err, tc.ErrorIsNil)
	models, err := s.store.AllModels("new-controller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/new-model": testModelDetails,
	})
}

func (s *ModelsSuite) TestUpdateModelExistingControllerAndModelNewModel(c *tc.C) {
	testModelDetails := jujuclient.ModelDetails{
		ModelUUID: "test.uuid",
		ModelType: model.IAAS,
	}
	err := s.store.UpdateModel("kontroll", "admin/new-model", testModelDetails)
	c.Assert(err, tc.ErrorIsNil)
	models, err := s.store.AllModels("kontroll")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/admin":     kontrollAdminModelDetails,
		"admin/my-model":  kontrollMyModelModelDetails,
		"admin/new-model": testModelDetails,
	})
}

func (s *ModelsSuite) TestUpdateModelOverwrites(c *tc.C) {
	testModelDetails := jujuclient.ModelDetails{
		ModelUUID: "test.uuid",
		ModelType: model.IAAS,
	}
	for i := 0; i < 2; i++ {
		// Twice so we exercise the code path of updating with
		// identical details.
		err := s.store.UpdateModel("kontroll", "admin/admin", testModelDetails)
		c.Assert(err, tc.ErrorIsNil)
		details, err := s.store.ModelByName("kontroll", "admin/admin")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(*details, tc.DeepEquals, testModelDetails)
	}
}

func (s *ModelsSuite) TestUpdateModelRejectsTypeChange(c *tc.C) {
	testModelDetails := jujuclient.ModelDetails{ModelUUID: "test.uuid", ModelType: model.CAAS}
	err := s.store.UpdateModel("kontroll", "admin/my-model", testModelDetails)
	c.Assert(err, tc.ErrorMatches, `model type was "iaas", cannot change to "caas"`)
}

func (s *ModelsSuite) TestUpdateModelEmptyModels(c *tc.C) {
	// This test exists to exercise a bug caused by the
	// presence of a file with an empty "models" field,
	// that would lead to a panic.
	err := os.WriteFile(jujuclient.JujuModelsPath(), []byte(`
controllers:
  ctrl:
    models:
`[1:]), 0644)
	c.Assert(err, tc.ErrorIsNil)

	testModelDetails := jujuclient.ModelDetails{
		ModelUUID: "test.uuid",
		ModelType: model.IAAS,
	}
	err = s.store.UpdateModel("ctrl", "admin/admin", testModelDetails)
	c.Assert(err, tc.ErrorIsNil)
	models, err := s.store.AllModels("ctrl")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/admin": testModelDetails,
	})
}

func (s *ModelsSuite) TestRemoveModelNoFile(c *tc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, tc.ErrorIsNil)
	err = s.store.RemoveModel("not-found", "admin/admin")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveModelControllerNotFound(c *tc.C) {
	err := s.store.RemoveModel("not-found", "admin/admin")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveModelNotFound(c *tc.C) {
	err := s.store.RemoveModel("kontroll", "admin/not-found")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveModel(c *tc.C) {
	err := s.store.RemoveModel("kontroll", "admin/admin")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.store.ModelByName("kontroll", "admin/admin")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ModelsSuite) TestRemoveControllerRemovesModels(c *tc.C) {
	store := jujuclient.NewFileClientStore()
	err := store.AddController("kontroll", jujuclient.ControllerDetails{
		ControllerUUID: "abc",
		CACert:         "woop",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = store.RemoveController("kontroll")
	c.Assert(err, tc.ErrorIsNil)

	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, tc.ErrorIsNil)
	_, ok := models["admin/kontroll"]
	c.Assert(ok, tc.IsFalse) // kontroll models are removed
}
