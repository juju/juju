// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/description"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type InternalSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&InternalSuite{})

func (s *InternalSuite) TestControllerValues(c *gc.C) {
	config := testing.ModelConfig(c)
	fields := controllerValues(config)
	c.Assert(fields, jc.DeepEquals, map[string]interface{}{
		"state-port": 19034,
		"api-port":   17777,
		"ca-cert":    testing.CACert,
	})
}

func (s *InternalSuite) TestUpdateConfigFromProvider(c *gc.C) {
	controllerConfig := testing.ModelConfig(c)
	configAttrs := testing.FakeConfig()
	configAttrs["type"] = "dummy"
	// Fake the "state-id" so the provider thinks it is prepared already.
	configAttrs["state-id"] = "42"
	// We need to specify a valid provider type, so we use dummy.
	// The dummy provider grabs the UUID from the controller config
	// and returns it in the map with the key "controller-uuid", similar
	// to what the azure provider will need to do.
	model := description.NewModel(description.ModelArgs{
		Owner:  names.NewUserTag("test-admin"),
		Config: configAttrs,
	})

	err := updateConfigFromProvider(model, controllerConfig)
	c.Assert(err, jc.ErrorIsNil)

	modelConfig := model.Config()
	controllerUUID, ok := controllerConfig.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(modelConfig["controller-uuid"], gc.Equals, controllerUUID)
}
