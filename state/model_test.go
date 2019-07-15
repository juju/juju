// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"github.com/juju/clock"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/crossmodel"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type ModelSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelSuite{})

func (s *ModelSuite) TestModel(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model.IsControllerModel(), jc.IsTrue)

	expectedTag := names.NewModelTag(model.UUID())
	c.Assert(model.Tag(), gc.Equals, expectedTag)
	c.Assert(model.ControllerTag(), gc.Equals, s.State.ControllerTag())
	c.Assert(model.Name(), gc.Equals, "testmodel")
	c.Assert(model.Owner(), gc.Equals, s.Owner)
	c.Assert(model.Life(), gc.Equals, state.Alive)
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeNone)
}

func (s *ModelSuite) TestModelDestroy(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestModelDestroyWithoutVolumes(c *gc.C) {
	//https://bugs.launchpad.net/juju/+bug/1800872
	// Models introduced in 2.1 and then upgraded to 2.2 don't have Volumes or Filesystem attributes
	// on their modelEntitiesRefs documents
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelEntities, closer := state.GetCollection(s.State, state.ModelEntityRefsC)
	defer closer()
	rawModelEntities := modelEntities.Writeable().Underlying()
	err = rawModelEntities.Update(bson.M{"_id": model.UUID()}, bson.M{"$unset": bson.M{"volumes": 1, "filesystems": 1}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestNewModelNonExistentLocalUser(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("non-existent@local")

	_, _, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `cannot create model: user "non-existent" not found`)
}

func (s *ModelSuite) TestNewModelSameUserSameNameFails(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	model, st1, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()
	c.Assert(model.UniqueIndexExists(), jc.IsTrue)

	// Attempt to create another model with a different UUID but the
	// same owner and name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg2 := testing.CustomModelConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})
	_, _, err = s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	errMsg := fmt.Sprintf("model %q for %s already exists", cfg2.Name(), owner.Id())
	c.Assert(err, gc.ErrorMatches, errMsg)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)

	// Remove the first model.
	model1, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model1.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Destroy only sets the model to dying and RemoveDyingModel can
	// only be called on a dead model. Normally, the environ's lifecycle
	// would be set to dead after machines and applications have been cleaned up.
	err = model1.SetDead()
	c.Assert(err, jc.ErrorIsNil)
	err = st1.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)

	// We should now be able to create the other model.
	model2, st2, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st2.Close()
	c.Assert(model2, gc.NotNil)
	c.Assert(st2, gc.NotNil)
}

func (s *ModelSuite) TestNewCAASModelDifferentUser(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()
	owner2 := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	model, st1, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()
	c.Assert(model.UniqueIndexExists(), jc.IsTrue)

	// Attempt to create another model with a different UUID and owner
	// but the name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg2 := testing.CustomModelConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})

	// We should now be able to create the other model.
	model2, st2, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner2,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st2.Close()
	c.Assert(model2.UniqueIndexExists(), jc.IsTrue)
}

func (s *ModelSuite) TestNewCAASModelSameUserFails(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	model, st1, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()
	c.Assert(model.UniqueIndexExists(), jc.IsTrue)

	// Attempt to create another model with a different UUID but the
	// same owner and name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg2 := testing.CustomModelConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})
	_, _, err = s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	errMsg := fmt.Sprintf("model %q for %s already exists", cfg2.Name(), owner.Name())
	c.Assert(err, gc.ErrorMatches, errMsg)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)

	// Remove the first model.
	model1, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model1.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Destroy only sets the model to dying and RemoveDyingModel can
	// only be called on a dead model. Normally, the environ's lifecycle
	// would be set to dead after machines and applications have been cleaned up.
	err = model1.SetDead()
	c.Assert(err, jc.ErrorIsNil)
	err = st1.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)

	// We should now be able to create the other model.
	model2, st2, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st2.Close()
	c.Assert(model2, gc.NotNil)
	c.Assert(st2, gc.NotNil)
}

func (s *ModelSuite) TestNewModelMissingType(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")
	_, _, err := s.Controller.NewModel(state.ModelArgs{
		// No type
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, "empty Type not valid")

}

func (s *ModelSuite) TestNewModel(c *gc.C) {
	cfg, uuid := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model.IsControllerModel(), jc.IsFalse)
	defer st.Close()

	modelTag := names.NewModelTag(uuid)
	assertModelMatches := func(model *state.Model) {
		c.Assert(model.UUID(), gc.Equals, modelTag.Id())
		c.Assert(model.Type(), gc.Equals, state.ModelTypeIAAS)
		c.Assert(model.Tag(), gc.Equals, modelTag)
		c.Assert(model.ControllerTag(), gc.Equals, s.State.ControllerTag())
		c.Assert(model.Owner(), gc.Equals, owner)
		c.Assert(model.Name(), gc.Equals, "testing")
		c.Assert(model.Life(), gc.Equals, state.Alive)
		c.Assert(model.CloudRegion(), gc.Equals, "dummy-region")
	}
	assertModelMatches(model)

	model, ph, err := s.StatePool.GetModel(uuid)
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	assertModelMatches(model)

	model, err = st.Model()
	c.Assert(err, jc.ErrorIsNil)
	assertModelMatches(model)

	// Check that the cloud's model count is incremented.
	testCloud, err := s.State.Cloud("dummy")
	c.Assert(err, jc.ErrorIsNil)
	refCount, err := state.CloudModelRefCount(st, testCloud.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(refCount, gc.Equals, 2)

	// Since the model tag for the State connection is different,
	// asking for this model through FindEntity returns a not found error.
	_, err = s.State.FindEntity(modelTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	entity, err := st.FindEntity(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, modelTag)

	// Ensure the model is functional by adding a machine
	_, err = st.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the default model was created.
	_, err = st.Space(corenetwork.DefaultSpaceName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSuite) TestNewModelRegionNameEscaped(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dotty.region",
		Config:                  cfg,
		Owner:                   names.NewUserTag("test@remote"),
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	c.Assert(model.CloudRegion(), gc.Equals, "dotty.region")
}

func (s *ModelSuite) TestNewModelImportingMode(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		MigrationMode:           state.MigrationModeImporting,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeImporting)
}

func (s *ModelSuite) TestSetMigrationMode(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeExporting)
}

func (s *ModelSuite) TestModelExists(c *gc.C) {
	modelExists, err := s.State.ModelExists(s.State.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelExists, jc.IsTrue)
}

func (s *ModelSuite) TestModelExistsNoModel(c *gc.C) {
	modelExists, err := s.State.ModelExists("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelExists, jc.IsFalse)
}

func (s *ModelSuite) TestSLA(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	level, err := st.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(level, gc.Equals, "unsupported")
	c.Assert(model.SLACredential(), gc.DeepEquals, []byte{})
	for _, goodLevel := range []string{"unsupported", "essential", "standard", "advanced"} {
		err = st.SetSLA(goodLevel, "bob", []byte("auth "+goodLevel))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(model.Refresh(), jc.ErrorIsNil)
		level, err = st.SLALevel()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(level, gc.Equals, goodLevel)
		c.Assert(model.SLALevel(), gc.Equals, goodLevel)
		c.Assert(model.SLAOwner(), gc.Equals, "bob")
		c.Assert(model.SLACredential(), gc.DeepEquals, []byte("auth "+goodLevel))
	}

	defaultLevel, err := state.NewSLALevel("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaultLevel, gc.Equals, state.SLAUnsupported)

	err = model.SetSLA("nope", "nobody", []byte("auth nope"))
	c.Assert(err, gc.ErrorMatches, `.*SLA level "nope" not valid.*`)

	c.Assert(model.SLALevel(), gc.Equals, "advanced")
	c.Assert(model.SLAOwner(), gc.Equals, "bob")
	c.Assert(model.SLACredential(), gc.DeepEquals, []byte("auth advanced"))
	slaCreds, err := st.SLACredential()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(slaCreds, gc.DeepEquals, []byte("auth advanced"))
}

func (s *ModelSuite) TestMeterStatus(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	ms, err := st.ModelMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms.Code, gc.Equals, state.MeterNotAvailable)
	c.Assert(ms.Info, gc.Equals, "")

	for i, validStatus := range []string{"RED", "GREEN", "AMBER"} {
		info := fmt.Sprintf("info setting %d", i)
		err = st.SetModelMeterStatus(validStatus, info)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(model.Refresh(), jc.ErrorIsNil)
		ms, err = st.ModelMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ms.Code.String(), gc.Equals, validStatus)
		c.Assert(ms.Info, gc.Equals, info)
	}

	err = model.SetMeterStatus("PURPLE", "foobar")
	c.Assert(err, gc.ErrorMatches, `meter status "PURPLE" not valid`)

	c.Assert(ms.Code, gc.Equals, state.MeterAmber)
	c.Assert(ms.Info, gc.Equals, "info setting 2")
}

func (s *ModelSuite) TestConfigForOtherModel(c *gc.C) {
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Name: "other"})
	defer otherState.Close()
	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Obtain another instance of the model via the StatePool
	model, ph, err := s.StatePool.GetModel(otherModel.UUID())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()

	conf, err := model.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.Name(), gc.Equals, "other")
	c.Assert(conf.UUID(), gc.Equals, otherModel.UUID())
}

func (s *ModelSuite) TestDeployCAASApplication(c *gc.C) {
	ch := s.Factory.MakeCharm(c, nil)
	args := state.AddApplicationArgs{
		Name:   "gitlab",
		Series: "kubernetes",
		Charm:  ch,
	}
	_, err := s.State.AddApplication(args)
	c.Assert(err, gc.ErrorMatches, `cannot add application "gitlab": series "kubernetes" in a non container model not valid`)
}

func (s *ModelSuite) TestAllUnits(c *gc.C) {
	wordpress := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})
	mysql := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "mysql",
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	units, err := model.AllUnits()
	c.Assert(err, jc.ErrorIsNil)

	var unitNames []string
	for _, u := range units {
		if !u.ShouldBeAssigned() {
			c.Fail()
		}
		unitNames = append(unitNames, u.Name())
	}
	sort.Strings(unitNames)
	c.Assert(unitNames, jc.DeepEquals, []string{
		"mysql/0", "wordpress/0", "wordpress/1",
	})
}

func (s *ModelSuite) TestAllEndpointBindings(c *gc.C) {
	type mockApplicationEndpointBindings struct {
		appName string
		binding map[string]string
	}

	s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})
	state.AddTestingApplicationWithBindings(
		c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"),
		map[string]string{"db": "one"})

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	listBindings, err := model.AllEndpointBindings()
	c.Assert(listBindings, gc.HasLen, 1)

	c.Assert(listBindings[0], jc.DeepEquals, state.ApplicationEndpointBindings{
		AppName: "wordpress",
		Bindings: map[string]string{
			"cache":           "",
			"foo-bar":         "",
			"db-client":       "",
			"admin-api":       "",
			"url":             "",
			"logging-dir":     "",
			"monitoring-port": "",
			"db":              "one",
		},
	})
}

// createTestModelConfig returns a new model config and its UUID for testing.
func (s *ModelSuite) createTestModelConfig(c *gc.C) (*config.Config, string) {
	return createTestModelConfig(c, s.modelTag.Id())
}

func createTestModelConfig(c *gc.C, controllerUUID string) (*config.Config, string) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	if controllerUUID == "" {
		controllerUUID = uuid.String()
	}
	return testing.CustomModelConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	}), uuid.String()
}

func (s *ModelSuite) TestModelConfigSameModelAsState(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := model.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.UUID(), gc.Equals, s.State.ModelUUID())
}

func (s *ModelSuite) TestModelConfigDifferentModelThanState(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	model, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := model.Config()
	c.Assert(err, jc.ErrorIsNil)
	uuid := cfg.UUID()
	c.Assert(uuid, gc.Equals, model.UUID())
	c.Assert(uuid, gc.Not(gc.Equals), s.State.ModelUUID())
}

func (s *ModelSuite) TestDestroyControllerModel(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyOtherModel(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	model, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)
	c.Assert(st2.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.Satisfies, errors.IsNotFound)
	// Destroying an empty model also removes the name index doc.
	c.Assert(model.UniqueIndexExists(), jc.IsFalse)
}

func (s *ModelSuite) TestDestroyControllerNonEmptyModelFails(c *gc.C) {
	s.assertDestroyControllerNonEmptyModelFails(c, nil)
}

func (s *ModelSuite) TestDestroyControllerNonEmptyModelWithForceFails(c *gc.C) {
	force := true
	s.assertDestroyControllerNonEmptyModelFails(c, &force)
}

func (s *ModelSuite) assertDestroyControllerNonEmptyModelFails(c *gc.C, force *bool) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	factory.NewFactory(st2, s.StatePool).MakeApplication(c, nil)

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{Force: force}), gc.ErrorMatches, "failed to destroy model: hosting 1 other model")
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Alive)
	model2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model2.Refresh(), jc.ErrorIsNil)
	c.Assert(model2.Life(), gc.Equals, state.Alive)
}

func (s *ModelSuite) TestDestroyControllerWithEmptyModel(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), jc.ErrorIsNil)
	c.Assert(controllerModel.Life(), gc.Equals, state.Dying)
	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	hostedModel, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostedModel.Refresh(), jc.ErrorIsNil)
	c.Logf("model %s, life %s", hostedModel.UUID(), hostedModel.Life())
	c.Assert(hostedModel.Life(), gc.Equals, state.Dying)
	c.Assert(st2.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(hostedModel.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModels(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	factory.NewFactory(st2, s.StatePool).MakeApplication(c, nil)

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), jc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	// Cleanups for hosted model enqueued by controller model cleanups.
	assertNeedsCleanup(c, st2)
	assertCleanupRuns(c, st2)

	model2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model2.Life(), gc.Equals, state.Dying)

	c.Assert(st2.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(st2.RemoveDyingModel(), jc.ErrorIsNil)

	c.Assert(model2.Refresh(), jc.Satisfies, errors.IsNotFound)

	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModelsWithResources(c *gc.C) {
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	assertModel := func(model *state.Model, st *state.State, life state.Life, expectedMachines, expectedApplications int) {
		c.Assert(model.Refresh(), jc.ErrorIsNil)
		c.Assert(model.Life(), gc.Equals, life)

		machines, err := st.AllMachines()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machines, gc.HasLen, expectedMachines)

		applications, err := st.AllApplications()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(applications, gc.HasLen, expectedApplications)
	}

	// add some machines and applications
	otherModel, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherSt.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	ch, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)

	args := state.AddApplicationArgs{
		Name:  application.Name(),
		Charm: ch,
	}
	application, err = otherSt.AddApplication(args)
	c.Assert(err, jc.ErrorIsNil)

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), jc.ErrorIsNil)

	assertCleanupCount(c, s.State, 2)
	assertAllMachinesDeadAndRemove(c, s.State)
	assertModel(controllerModel, s.State, state.Dying, 0, 0)

	err = s.State.ProcessDyingModel()
	c.Assert(err, jc.Satisfies, state.IsHasHostedModelsError)
	c.Assert(err, gc.ErrorMatches, `hosting 1 other model`)

	assertCleanupCount(c, otherSt, 4)
	assertAllMachinesDeadAndRemove(c, otherSt)
	assertModel(otherModel, otherSt, state.Dying, 0, 0)
	c.Assert(otherSt.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(otherSt.RemoveDyingModel(), jc.ErrorIsNil)

	c.Assert(otherModel.Refresh(), jc.Satisfies, errors.IsNotFound)

	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) assertDestroyControllerAndHostedModelsWithPersistentStorage(c *gc.C, force *bool) {
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	// Add a unit with persistent storage, which will prevent Destroy
	// from succeeding on account of DestroyStorage being nil.
	otherFactory := factory.NewFactory(otherSt, s.StatePool)
	otherFactory.MakeUnit(c, &factory.UnitParams{
		Application: otherFactory.MakeApplication(c, &factory.ApplicationParams{
			Charm: otherFactory.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
				URL:  "cs:quantal/storage-block-1",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped"},
			},
		}),
	})

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		Force:               force,
	})
	c.Assert(err, jc.Satisfies, state.IsHasPersistentStorageError)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModelsWithPersistentStorage(c *gc.C) {
	s.assertDestroyControllerAndHostedModelsWithPersistentStorage(c, nil)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModelsWithPersistentStorageWithForce(c *gc.C) {
	force := true
	s.assertDestroyControllerAndHostedModelsWithPersistentStorage(c, &force)
}

func (s *ModelSuite) TestDestroyControllerEmptyModelRace(c *gc.C) {
	defer s.Factory.MakeModel(c, nil).Close()

	// Simulate an empty model being added just before the
	// remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.Factory.MakeModel(c, nil).Close()
	}).Check()

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyControllerRemoveEmptyAddNonEmptyModel(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	// Simulate an empty model being removed, and a new non-empty
	// model being added, just before the remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		// Destroy the empty model, which should move it right
		// along to Dead, and then remove it.
		model, err := st2.Model()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
		err = st2.RemoveDyingModel()
		c.Assert(err, jc.ErrorIsNil)

		// Add a new, non-empty model. This should still prevent
		// the controller from being destroyed.
		st3 := s.Factory.MakeModel(c, nil)
		defer st3.Close()
		factory.NewFactory(st3, s.StatePool).MakeApplication(c, nil)
	}).Check()

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), gc.ErrorMatches, "failed to destroy model: hosting 1 other model")
}

func (s *ModelSuite) TestDestroyControllerNonEmptyModelRace(c *gc.C) {
	// Simulate an empty model being added just before the
	// remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		st := s.Factory.MakeModel(c, nil)
		defer st.Close()
		factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)
	}).Check()

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), gc.ErrorMatches, "failed to destroy model: hosting 1 other model")
}

func (s *ModelSuite) TestDestroyControllerAlreadyDyingRaceNoOp(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Simulate an model being destroyed by another client just before
	// the remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	}).Check()

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyControllerAlreadyDyingNoOp(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyModelNonEmpty(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Add a application to prevent the model from transitioning directly to Dead.
	s.Factory.MakeApplication(c, nil)

	c.Assert(m.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)

	// Since the model is only dying and not dead, the unique index is still there.
	c.Assert(m.UniqueIndexExists(), jc.IsTrue)
}

func (s *ModelSuite) assertDestroyModelPersistentStorage(c *gc.C, force *bool) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Add a unit with persistent storage, which will prevent Destroy
	// from succeeding on account of DestroyStorage being nil.
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped"},
			},
		}),
	})

	err = m.Destroy(state.DestroyModelParams{Force: force})
	c.Assert(err, jc.Satisfies, state.IsHasPersistentStorageError)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Alive)
}

func (s *ModelSuite) TestDestroyModelPersistentStorage(c *gc.C) {
	s.assertDestroyModelPersistentStorage(c, nil)
}

func (s *ModelSuite) TestDestroyModelPersistentStorageWithForce(c *gc.C) {
	force := true
	s.assertDestroyModelPersistentStorage(c, &force)
}

func (s *ModelSuite) TestDestroyModelNonPersistentStorage(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Add a unit with non-persistent storage, which should not prevent
	// Destroy from succeeding.
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "loop"},
			},
		}),
	})

	err = m.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelDestroyStorage(c *gc.C) {
	s.testDestroyModelDestroyStorage(c, true)
}

func (s *ModelSuite) TestDestroyModelReleaseStorage(c *gc.C) {
	s.testDestroyModelDestroyStorage(c, false)
}

func (s *ModelSuite) testDestroyModelDestroyStorage(c *gc.C, destroyStorage bool) {
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped"},
			},
		}),
	})

	err := s.Model.Destroy(state.DestroyModelParams{DestroyStorage: &destroyStorage})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.Model.Refresh(), jc.ErrorIsNil)
	c.Assert(s.Model.Life(), gc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State) // destroy application
	assertCleanupRuns(c, s.State) // destroy unit
	assertCleanupRuns(c, s.State) // destroy/release storage

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	volume, err := sb.Volume(names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volume.Life(), gc.Equals, state.Dying)
	c.Assert(volume.Releasing(), gc.Equals, !destroyStorage)
}

func (s *ModelSuite) assertDestroyModelReleaseStorageUnreleasable(c *gc.C, force *bool) {
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped-unreleasable"},
			},
		}),
	})

	destroyStorage := false
	err := s.Model.Destroy(state.DestroyModelParams{DestroyStorage: &destroyStorage, Force: force})
	expectedErr := fmt.Sprintf(`failed to destroy model: cannot release volume 0: ` +
		`storage provider "modelscoped-unreleasable" does not support releasing storage`)
	c.Assert(err, gc.ErrorMatches, expectedErr)
	c.Assert(s.Model.Refresh(), jc.ErrorIsNil)
	c.Assert(s.Model.Life(), gc.Equals, state.Alive)
	assertDoesNotNeedCleanup(c, s.State)
}

func (s *ModelSuite) TestDestroyModelReleaseStorageUnreleasable(c *gc.C) {
	s.assertDestroyModelReleaseStorageUnreleasable(c, nil)
}

func (s *ModelSuite) TestDestroyModelReleaseStorageUnreleasableWithForce(c *gc.C) {
	force := true
	s.assertDestroyModelReleaseStorageUnreleasable(c, &force)
}

func (s *ModelSuite) TestDestroyModelAddApplicationConcurrently(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, st, func() {
		factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)
	}).Check()

	c.Assert(m.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelAddMachineConcurrently(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, st, func() {
		factory.NewFactory(st, s.StatePool).MakeMachine(c, nil)
	}).Check()

	c.Assert(m.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelEmpty(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(m.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
	c.Assert(st.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestDestroyModelWithApplicationOffers(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	app := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))

	ao := state.NewApplicationOffers(s.State)
	offer, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = m.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)

	// Run the cleanups, check that the application and offer are
	// both removed.
	assertCleanupCount(c, s.State, 2)

	_, err = ao.ApplicationOffer(offer.OfferName)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = app.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestForceDestroySetsForceDestroyed(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.ForceDestroyed(), gc.Equals, false)

	force := true
	err = model.Destroy(state.DestroyModelParams{
		Force: &force,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Life(), gc.Equals, state.Dying)
	c.Assert(model.ForceDestroyed(), gc.Equals, true)
}

func (s *ModelSuite) TestNonForceDestroy(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	noForce := false
	err = model.Destroy(state.DestroyModelParams{
		Force: &noForce,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Life(), gc.Equals, state.Dying)
	c.Assert(model.ForceDestroyed(), gc.Equals, false)
}

func (s *ModelSuite) TestProcessDyingServerModelTransitionDyingToDead(c *gc.C) {
	s.assertDyingModelTransitionDyingToDead(c, s.State)
}

func (s *ModelSuite) TestProcessDyingHostedModelTransitionDyingToDead(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	s.assertDyingModelTransitionDyingToDead(c, st)
}

func (s *ModelSuite) assertDyingModelTransitionDyingToDead(c *gc.C, st *state.State) {
	// Add a application to prevent the model from transitioning directly to Dead.
	// Add the application before getting the Model, otherwise we'll have to run
	// the transaction twice, and hit the hook point too early.
	app := factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	// ProcessDyingModel is called by a worker after Destroy is called. To
	// avoid a race, we jump the gun here and test immediately after the
	// environement was set to dead.
	defer state.SetAfterHooks(c, st, func() {
		c.Assert(model.Refresh(), jc.ErrorIsNil)
		c.Assert(model.Life(), gc.Equals, state.Dying)

		err := app.Destroy()
		c.Assert(err, jc.ErrorIsNil)

		c.Check(model.UniqueIndexExists(), jc.IsTrue)
		c.Assert(st.ProcessDyingModel(), jc.ErrorIsNil)
		c.Assert(st.RemoveDyingModel(), jc.ErrorIsNil)

		c.Assert(model.Refresh(), jc.Satisfies, errors.IsNotFound)
		c.Check(model.UniqueIndexExists(), jc.IsFalse)
	}).Check()

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingModelWithMachinesAndApplicationsNoOp(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	// calling ProcessDyingModel on a live environ should fail.
	err := st.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")

	// add some machines and applications
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	ch, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	args := state.AddApplicationArgs{
		Name:  application.Name(),
		Charm: ch,
	}
	application, err = st.AddApplication(args)
	c.Assert(err, jc.ErrorIsNil)

	assertModel := func(life state.Life, expectedMachines, expectedApplications int) {
		c.Assert(model.Refresh(), jc.ErrorIsNil)
		c.Assert(model.Life(), gc.Equals, life)

		machines, err := st.AllMachines()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machines, gc.HasLen, expectedMachines)

		applications, err := st.AllApplications()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(applications, gc.HasLen, expectedApplications)
	}

	// Simulate processing a dying model after an model is set to
	// dying, but before the cleanup has removed machines and applications.
	defer state.SetAfterHooks(c, st, func() {
		assertModel(state.Dying, 1, 1)
		err := st.ProcessDyingModel()
		c.Assert(err, jc.Satisfies, state.IsModelNotEmptyError)
		c.Assert(err, gc.ErrorMatches, `model not empty, found 1 machine, 1 application`)
	}).Check()

	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingModelWithVolumeBackedFilesystems(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{
				Pool: "modelscoped-block",
				Size: 123,
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)
	filesystems, err := sb.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystems, gc.HasLen, 1)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.Satisfies, state.IsHasPersistentStorageError)

	destroyStorage := true
	c.Assert(model.Destroy(state.DestroyModelParams{
		DestroyStorage: &destroyStorage,
	}), jc.ErrorIsNil)

	err = sb.DetachFilesystem(machine.MachineTag(), names.NewFilesystemTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	err = sb.RemoveFilesystemAttachment(machine.MachineTag(), names.NewFilesystemTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	err = sb.DetachVolume(machine.MachineTag(), names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	err = sb.RemoveVolumeAttachment(machine.MachineTag(), names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	c.Assert(machine.Remove(), jc.ErrorIsNil)

	// The filesystem will be gone, but the volume is persistent and should
	// not have been removed.
	err = st.ProcessDyingModel()
	c.Assert(err, jc.Satisfies, state.IsModelNotEmptyError)
	c.Assert(err, gc.ErrorMatches, `model not empty, found 1 volume, 1 filesystem`)
}

func (s *ModelSuite) TestProcessDyingModelWithVolumes(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "modelscoped",
				Size: 123,
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)
	volumes, err := sb.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 1)
	volumeTag := volumes[0].VolumeTag()

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.Satisfies, state.IsHasPersistentStorageError)

	destroyStorage := true
	c.Assert(model.Destroy(state.DestroyModelParams{
		DestroyStorage: &destroyStorage,
	}), jc.ErrorIsNil)

	err = sb.DetachVolume(machine.MachineTag(), volumeTag)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.RemoveVolumeAttachment(machine.MachineTag(), volumeTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	c.Assert(machine.Remove(), jc.ErrorIsNil)

	// The volume is persistent and should not have been removed along with
	// the machine it was attached to.
	err = st.ProcessDyingModel()
	c.Assert(err, jc.Satisfies, state.IsModelNotEmptyError)
	c.Assert(err, gc.ErrorMatches, `model not empty, found 1 volume`)
}

func (s *ModelSuite) TestProcessDyingControllerModelWithHostedModelsNoOp(c *gc.C) {
	// Add a non-empty model to the controller.
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
	}), jc.ErrorIsNil)

	err = s.State.ProcessDyingModel()
	c.Assert(err, jc.Satisfies, state.IsHasHostedModelsError)
	c.Assert(err, gc.ErrorMatches, `hosting 1 other model`)

	c.Assert(controllerModel.Refresh(), jc.ErrorIsNil)
	c.Assert(controllerModel.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestListModelUsers(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	expected := s.addModelUsers(c, s.State)
	obtained, err := model.Users()
	c.Assert(err, gc.IsNil)

	assertObtainedUsersMatchExpectedUsers(c, obtained, expected)
}

func (s *ModelSuite) TestListUsersIgnoredDeletedUsers(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	expectedUsers := s.addModelUsers(c, s.State)

	obtainedUsers, err := model.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedUsers)

	lastUser := obtainedUsers[len(obtainedUsers)-1]
	err = s.State.RemoveUser(lastUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	expectedAfterDeletion := obtainedUsers[:len(obtainedUsers)-1]

	obtainedUsers, err = model.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedAfterDeletion)
}

func (s *ModelSuite) TestListUsersTwoModels(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	otherModelState := s.Factory.MakeModel(c, nil)
	defer otherModelState.Close()
	otherModel, err := otherModelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Add users to both models
	expectedUsers := s.addModelUsers(c, s.State)
	expectedUsersOtherModel := s.addModelUsers(c, otherModelState)

	// test that only the expected users are listed for each model
	obtainedUsers, err := model.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedUsers)

	obtainedUsersOtherModel, err := otherModel.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsersOtherModel, expectedUsersOtherModel)

	// It doesn't matter how you obtain the Model.
	otherModel2, ph, err := s.StatePool.GetModel(otherModel.UUID())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	obtainedUsersOtherModel2, err := otherModel2.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsersOtherModel2, expectedUsersOtherModel)
}

func (s *ModelSuite) addModelUsers(c *gc.C, st *state.State) (expected []permission.UserAccess) {
	// get the model owner
	testAdmin := names.NewUserTag("test-admin")
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	owner, err := st.UserAccess(testAdmin, m.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(st, s.StatePool)
	return []permission.UserAccess{
		// we expect the owner to be an existing model user
		owner,
		// add new users to the model
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
	}
}

func assertObtainedUsersMatchExpectedUsers(c *gc.C, obtainedUsers, expectedUsers []permission.UserAccess) {
	c.Assert(len(obtainedUsers), gc.Equals, len(expectedUsers))
	expectedByUser := make(map[string]permission.UserAccess, len(expectedUsers))
	for _, access := range expectedUsers {
		expectedByUser[access.UserName] = access
	}
	for _, obtained := range obtainedUsers {
		expect := expectedByUser[obtained.UserName]
		// We shouldn't get the same entry again
		delete(expectedByUser, obtained.UserName)
		c.Check(obtained.Object.Id(), gc.Equals, expect.Object.Id())
		c.Check(obtained.UserTag, gc.Equals, expect.UserTag)
		c.Check(obtained.DisplayName, gc.Equals, expect.DisplayName)
		c.Check(obtained.CreatedBy, gc.Equals, expect.CreatedBy)
	}
	c.Check(expectedByUser, jc.DeepEquals, map[string]permission.UserAccess{})
}

func (s *ModelSuite) TestAllModelUUIDs(c *gc.C) {
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	obtained, err := s.State.AllModelUUIDs()
	c.Assert(err, jc.ErrorIsNil)
	expected := []string{
		s.State.ModelUUID(),
		st1.ModelUUID(),
		st2.ModelUUID(),
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *ModelSuite) TestAllModelUUIDsExcludesDead(c *gc.C) {
	expected := []string{
		s.State.ModelUUID(),
	}

	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()

	m1, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	expectedWithAddition := append(expected, m1.UUID())
	obtained, err := s.State.AllModelUUIDs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, jc.DeepEquals, expectedWithAddition)

	err = m1.SetDead()
	c.Assert(err, jc.ErrorIsNil)

	obtained, err = s.State.AllModelUUIDs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, jc.DeepEquals, expected)

	obtained, err = s.State.AllModelUUIDsIncludingDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, jc.DeepEquals, expectedWithAddition)
}

func (s *ModelSuite) TestHostedModelCount(c *gc.C) {
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)

	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 1)

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 2)

	model1, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 1)

	model2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model2.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(st2.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)
}

func (s *ModelSuite) TestNewModelEnvironVersion(c *gc.C) {
	v := 123
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		EnvironVersion: v,
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.EnvironVersion(), gc.Equals, v)
}

func (s *ModelSuite) TestSetEnvironVersion(c *gc.C) {
	v := 123
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		m, err := s.State.Model()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.EnvironVersion(), gc.Equals, 0)
		err = m.SetEnvironVersion(v)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.EnvironVersion(), gc.Equals, v)
	}).Check()

	err = m.SetEnvironVersion(v)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.EnvironVersion(), gc.Equals, v)
}

func (s *ModelSuite) TestSetEnvironVersionCannotDecrease(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		m, err := s.State.Model()
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetEnvironVersion(2)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.EnvironVersion(), gc.Equals, 2)
	}).Check()

	err = m.SetEnvironVersion(1)
	c.Assert(err, gc.ErrorMatches, `cannot set environ version to 1, which is less than the current version 2`)
	// m's cached version is only updated on success
	c.Assert(m.EnvironVersion(), gc.Equals, 0)

	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.EnvironVersion(), gc.Equals, 2)
}

type ModelCloudValidationSuite struct {
	gitjujutesting.MgoSuite
}

var _ = gc.Suite(&ModelCloudValidationSuite{})

// TODO(axw) concurrency tests when we can modify the cloud definition,
// and update/remove credentials.

func (s *ModelCloudValidationSuite) TestNewModelDifferentCloud(c *gc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "some-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st := controller.SystemState()
	err := st.AddCloud(cloud.Cloud{
		Name:      "another",
		Type:      "dummy",
		AuthTypes: cloud.AuthTypes{"empty", "userpass"},
	}, owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	cfg, err = cfg.Apply(map[string]interface{}{"name": "whatever"})
	c.Assert(err, jc.ErrorIsNil)
	m, newSt, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "another",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()
	c.Assert(m.Cloud(), gc.Equals, "another")
}

func (s *ModelCloudValidationSuite) TestNewModelUnknownCloudRegion(c *gc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "some-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st := controller.SystemState()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `region "dummy-region" not found \(expected one of \["some-region"\]\)`)
}

func (s *ModelCloudValidationSuite) TestNewModelDefaultCloudRegion(c *gc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "dummy-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st := controller.SystemState()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	cfg, err := cfg.Apply(map[string]interface{}{"name": "whatever"})
	c.Assert(err, jc.ErrorIsNil)
	m, newSt, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()
	c.Assert(m.CloudRegion(), gc.Equals, "dummy-region")
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudRegion(c *gc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "dummy-region"}, {Name: "dummy-region2"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st := controller.SystemState()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, "missing CloudRegion not valid")
}

func (s *ModelCloudValidationSuite) TestNewModelUnknownCloudCredential(c *gc.C) {
	regions := []cloud.Region{{Name: "dummy-region"}}
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	controller, owner := s.initializeState(
		c, regions, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer controller.Close()
	st := controller.SystemState()
	unknownCredentialTag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/unknown-credential")
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		CloudCredential:         unknownCredentialTag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `credential "dummy/test@remote/unknown-credential" not found`)
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudCredential(c *gc.C) {
	regions := []cloud.Region{{Name: "dummy-region"}}
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	controller, owner := s.initializeState(
		c, regions, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer controller.Close()
	st := controller.SystemState()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, "missing CloudCredential not valid")
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudCredentialSupportsEmptyAuth(c *gc.C) {
	regions := []cloud.Region{
		{
			Name:             "dummy-region",
			Endpoint:         "dummy-endpoint",
			IdentityEndpoint: "dummy-identity-endpoint",
			StorageEndpoint:  "dummy-storage-endpoint",
		},
	}
	controller, owner := s.initializeState(c, regions, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st := controller.SystemState()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	cfg, err := cfg.Apply(map[string]interface{}{"name": "whatever"})
	c.Assert(err, jc.ErrorIsNil)
	_, newSt, err := controller.NewModel(state.ModelArgs{
		Type:      state.ModelTypeIAAS,
		CloudName: "dummy", CloudRegion: "dummy-region", Config: cfg, Owner: owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	newSt.Close()
}

func (s *ModelCloudValidationSuite) TestNewModelOtherUserCloudCredential(c *gc.C) {
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	controller, _ := s.initializeState(
		c, nil, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer controller.Close()
	st := controller.SystemState()
	owner := factory.NewFactory(st, controller.StatePool()).MakeUser(c, nil).UserTag()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		Config:                  cfg,
		Owner:                   owner,
		CloudCredential:         controllerCredentialTag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `credential "dummy/test@remote/controller-credential" not found`)
}

func (s *ModelCloudValidationSuite) initializeState(
	c *gc.C,
	regions []cloud.Region,
	authTypes []cloud.AuthType,
	credentials map[names.CloudCredentialTag]cloud.Credential,
) (*state.Controller, names.UserTag) {
	owner := names.NewUserTag("test@remote")
	cfg, _ := createTestModelConfig(c, "")
	var controllerRegion string
	var controllerCredential names.CloudCredentialTag
	if len(regions) > 0 {
		controllerRegion = regions[0].Name
	}
	if len(credentials) > 0 {
		// pick an arbitrary credential
		for controllerCredential = range credentials {
		}
	}
	controllerCfg := testing.FakeControllerConfig()
	controller, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			Owner:                   owner,
			Config:                  cfg,
			CloudName:               "dummy",
			CloudRegion:             controllerRegion,
			CloudCredential:         controllerCredential,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: authTypes,
			Regions:   regions,
		},
		CloudCredentials: credentials,
		MongoSession:     s.Session,
		AdminPassword:    "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	return controller, owner
}

func assertCleanupRuns(c *gc.C, st *state.State) {
	err := st.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
}

func assertNeedsCleanup(c *gc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsTrue)
}

func assertDoesNotNeedCleanup(c *gc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsFalse)
}

// assertCleanupCount is useful because certain cleanups cause other cleanups
// to be queued; it makes more sense to just run cleanup again than to unpick
// object destruction so that we run the cleanups inline while running cleanups.
func assertCleanupCount(c *gc.C, st *state.State, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		assertNeedsCleanup(c, st)
		assertCleanupRuns(c, st)
	}
	assertDoesNotNeedCleanup(c, st)
}

// The provisioner will remove dead machines once their backing instances are
// stopped. For the tests, we remove them directly.
func assertAllMachinesDeadAndRemove(c *gc.C, st *state.State) {
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		c.Assert(err, jc.ErrorIsNil)
		if manual {
			continue
		}

		c.Assert(m.Life(), gc.Equals, state.Dead)
		c.Assert(m.Remove(), jc.ErrorIsNil)
	}
}
