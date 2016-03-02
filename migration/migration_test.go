// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/description"
	"github.com/juju/juju/migration"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
)

type ImportSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&ImportSuite{})

func (s *ImportSuite) SetUpTest(c *gc.C) {
	// Specify the config to use for the controller model before calling
	// SetUpTest of the StateSuite, otherwise we get testing.ModelConfig(c).
	// The default provider type specified in the testing.ModelConfig function
	// is one that isn't registered as a valid provider. For our tests here we
	// need a real registered provider, so we use the dummy provider.
	// NOTE: make a better test provider.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"type":     "dummy",
		"state-id": "42",
	})
	s.StateSuite.SetUpTest(c)
}

func (s *ImportSuite) TestBadBytes(c *gc.C) {
	bytes := []byte("not a model")
	model, st, err := migration.ImportModel(s.State, bytes)
	c.Check(st, gc.IsNil)
	c.Check(model, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "yaml: unmarshal errors:\n.*")
}

func (s *ImportSuite) TestImportModel(c *gc.C) {
	model, err := s.State.Export()
	c.Check(err, jc.ErrorIsNil)

	controllerConfig, err := s.State.ModelConfig()
	c.Check(err, jc.ErrorIsNil)

	// Update the config values in the exported model for different values for
	// "state-port", "api-port", and "ca-cert". Also give the model a new UUID
	// and name so we can import it nicely.
	model.UpdateConfig(map[string]interface{}{
		"name":       "new-model",
		"uuid":       utils.MustNewUUID().String(),
		"state-port": 12345,
		"api-port":   54321,
		"ca-cert":    "not really a cert",
	})

	bytes, err := description.Serialize(model)
	c.Check(err, jc.ErrorIsNil)

	dbModel, dbState, err := migration.ImportModel(s.State, bytes)
	c.Check(err, jc.ErrorIsNil)
	defer dbState.Close()

	dbConfig, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	attrs := dbConfig.AllAttrs()
	c.Assert(attrs["state-port"], gc.Equals, controllerConfig.StatePort())
	c.Assert(attrs["api-port"], gc.Equals, controllerConfig.APIPort())
	cacert, ok := controllerConfig.CACert()
	c.Assert(ok, jc.IsTrue)
	c.Assert(attrs["ca-cert"], gc.Equals, cacert)
	uuid, ok := controllerConfig.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(attrs["controller-uuid"], gc.Equals, uuid)
}
