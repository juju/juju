// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"

	"github.com/juju/gomaasapi/v2"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
)

// Ensure MAAS provider supports the expected interfaces.
var (
	_ config.ConfigSchemaSource = (*EnvironProvider)(nil)
)

type configSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&configSuite{})

// newConfig creates a MAAS environment config from attributes.
func newConfig(values map[string]interface{}) (*maasModelConfig, error) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"name": "testmodel",
		"type": "maas",
	}).Merge(values)
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return nil, err
	}
	return providerInstance.newConfig(context.Background(), cfg)
}

func (s *configSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	mockGetController := func(gomaasapi.ControllerArgs) (gomaasapi.Controller, error) {
		return nil, gomaasapi.NewUnsupportedVersionError("oops")
	}
	s.PatchValue(&GetMAASController, mockGetController)
}

func (*configSuite) TestValidateUpcallsEnvironsConfigValidate(c *tc.C) {
	// The base Validate() function will not allow an environment to
	// change its name.  Trigger that error so as to prove that the
	// environment provider's Validate() calls the base Validate().
	oldCfg, err := newConfig(nil)
	c.Assert(err, jc.ErrorIsNil)
	newName := oldCfg.Name() + "-but-different"
	newCfg, err := oldCfg.Apply(map[string]interface{}{"name": newName})
	c.Assert(err, jc.ErrorIsNil)

	_, err = EnvironProvider{}.Validate(context.Background(), newCfg, oldCfg.Config)

	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, ".*cannot change name.*")
}

func (*configSuite) TestSchema(c *tc.C) {
	fields := providerInstance.Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, tc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], jc.DeepEquals, field)
	}
}
