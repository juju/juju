// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	details, err := s.store.ModelByName("not-found", "admin@local", "admin")
	c.Assert(err, gc.ErrorMatches, "models for controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *ModelsSuite) TestModelByNameControllerNotFound(c *gc.C) {
	details, err := s.store.ModelByName("not-found", "admin@local", "admin")
	c.Assert(err, gc.ErrorMatches, "models for controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *ModelsSuite) TestModelByNameAccountNotFound(c *gc.C) {
	details, err := s.store.ModelByName("ctrl", "not@found", "admin")
	c.Assert(err, gc.ErrorMatches, "models for account not@found on controller ctrl not found")
	c.Assert(details, gc.IsNil)
}

func (s *ModelsSuite) TestModelByNameModelNotFound(c *gc.C) {
	details, err := s.store.ModelByName("kontroll", "admin@local", "not-found")
	c.Assert(err, gc.ErrorMatches, "model kontroll:admin@local:not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *ModelsSuite) TestModelByName(c *gc.C) {
	details, err := s.store.ModelByName("kontroll", "admin@local", "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.NotNil)
	c.Assert(*details, jc.DeepEquals, testControllerModels["kontroll"].AccountModels["admin@local"].Models["admin"])
}

func (s *ModelsSuite) TestAllModelsNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	models, err := s.store.AllModels("not-found", "admin@local")
	c.Assert(err, gc.ErrorMatches, "models for controller not-found not found")
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelsSuite) TestAllModels(c *gc.C) {
	models, err := s.store.AllModels("kontroll", "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, testControllerModels["kontroll"].AccountModels["admin@local"].Models)
}

func (s *ModelsSuite) TestCurrentModel(c *gc.C) {
	current, err := s.store.CurrentModel("kontroll", "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, "my-model")
}

func (s *ModelsSuite) TestCurrentModelNotSet(c *gc.C) {
	_, err := s.store.CurrentModel("ctrl", "bob@local")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestCurrentModelControllerNotFound(c *gc.C) {
	_, err := s.store.CurrentModel("not-found", "admin@local")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestCurrentModelAccountNotFound(c *gc.C) {
	_, err := s.store.CurrentModel("kontroll", "not@found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestSetCurrentModelControllerNotFound(c *gc.C) {
	err := s.store.SetCurrentModel("not-found", "admin@local", "admin")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestSetCurrentModelAccountNotFound(c *gc.C) {
	err := s.store.SetCurrentModel("kontroll", "not@found", "admin")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestSetCurrentModelModelNotFound(c *gc.C) {
	err := s.store.SetCurrentModel("kontroll", "admin@local", "not-found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestSetCurrentModel(c *gc.C) {
	err := s.store.SetCurrentModel("kontroll", "admin@local", "admin")
	c.Assert(err, jc.ErrorIsNil)
	all, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all["kontroll"].AccountModels["admin@local"].CurrentModel, gc.Equals, "admin")
}

func (s *ModelsSuite) TestUpdateModelNewController(c *gc.C) {
	testModelDetails := jujuclient.ModelDetails{"test.uuid"}
	err := s.store.UpdateModel("new-controller", "new@account", "new-model", testModelDetails)
	c.Assert(err, jc.ErrorIsNil)
	models, err := s.store.AllModels("new-controller", "new@account")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, map[string]jujuclient.ModelDetails{
		"new-model": testModelDetails,
	})
}

func (s *ModelsSuite) TestUpdateModelExistingControllerAndModelNewModel(c *gc.C) {
	testModelDetails := jujuclient.ModelDetails{"test.uuid"}
	err := s.store.UpdateModel("kontroll", "admin@local", "new-model", testModelDetails)
	c.Assert(err, jc.ErrorIsNil)
	models, err := s.store.AllModels("kontroll", "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin":     kontrollAdminModelDetails,
		"my-model":  kontrollMyModelModelDetails,
		"new-model": testModelDetails,
	})
}

func (s *ModelsSuite) TestUpdateModelOverwrites(c *gc.C) {
	testModelDetails := jujuclient.ModelDetails{"test.uuid"}
	for i := 0; i < 2; i++ {
		// Twice so we exercise the code path of updating with
		// identical details.
		err := s.store.UpdateModel("kontroll", "admin@local", "admin", testModelDetails)
		c.Assert(err, jc.ErrorIsNil)
		details, err := s.store.ModelByName("kontroll", "admin@local", "admin")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*details, jc.DeepEquals, testModelDetails)
	}
}

func (s *ModelsSuite) TestRemoveModelNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.RemoveModel("not-found", "admin@local", "admin")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestRemoveModelControllerNotFound(c *gc.C) {
	err := s.store.RemoveModel("not-found", "admin@local", "admin")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestRemoveModelAccountNotFound(c *gc.C) {
	err := s.store.RemoveModel("kontroll", "not@found", "admin")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestRemoveModelNotFound(c *gc.C) {
	err := s.store.RemoveModel("kontroll", "admin@local", "not-found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestRemoveModel(c *gc.C) {
	err := s.store.RemoveModel("kontroll", "admin@local", "admin")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.ModelByName("kontroll", "admin@local", "admin")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelsSuite) TestRemoveControllerRemovesModels(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	err := store.RemoveController("kontroll")
	c.Assert(err, jc.ErrorIsNil)

	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	_, ok := models["kontroll"]
	c.Assert(ok, jc.IsFalse) // kontroll models are removed
}
