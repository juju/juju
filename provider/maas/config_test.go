// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

// Ensure MAAS provider supports the expected interfaces.
var (
	_ config.ConfigSchemaSource = (*maasEnvironProvider)(nil)
)

type configSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&configSuite{})

// copyAttrs copies values from src into dest.  If src contains a key that was
// already in dest, its value in dest will still be updated to the one from
// src.
func copyAttrs(src, dest map[string]interface{}) {
	for k, v := range src {
		dest[k] = v
	}
}

// newConfig creates a MAAS environment config from attributes.
func newConfig(values map[string]interface{}) (*maasModelConfig, error) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"name": "testenv",
		"type": "maas",
	}).Merge(values)
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return nil, err
	}
	return providerInstance.newConfig(cfg)
}

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	mockCapabilities := func(*gomaasapi.MAASObject, string) (set.Strings, error) {
		return set.NewStrings("network-deployment-ubuntu"), nil
	}
	s.PatchValue(&GetCapabilities, mockCapabilities)
	mockGetController := func(string, string) (gomaasapi.Controller, error) {
		return nil, gomaasapi.NewUnsupportedVersionError("oops")
	}
	s.PatchValue(&GetMAAS2Controller, mockGetController)
}

func (*configSuite) TestValidateUpcallsEnvironsConfigValidate(c *gc.C) {
	// The base Validate() function will not allow an environment to
	// change its name.  Trigger that error so as to prove that the
	// environment provider's Validate() calls the base Validate().
	oldCfg, err := newConfig(nil)
	c.Assert(err, jc.ErrorIsNil)
	newName := oldCfg.Name() + "-but-different"
	newCfg, err := oldCfg.Apply(map[string]interface{}{"name": newName})
	c.Assert(err, jc.ErrorIsNil)

	_, err = maasEnvironProvider{}.Validate(newCfg, oldCfg.Config)

	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*cannot change name.*")
}

func (*configSuite) TestSchema(c *gc.C) {
	fields := providerInstance.Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, gc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], jc.DeepEquals, field)
	}
}
