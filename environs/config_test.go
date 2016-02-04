// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/manual"
	"github.com/juju/juju/testing"
)

type suite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

// dummySampleConfig returns the dummy sample config without
// the controller configured.
// This function also exists in cloudconfig/userdata_test
// Maybe place it in dummy and export it?
func dummySampleConfig() testing.Attrs {
	return dummy.SampleConfig().Merge(testing.Attrs{
		"controller": false,
	})
}

func inMap(attrs testing.Attrs, attr string) bool {
	_, ok := attrs[attr]
	return ok
}

func (*suite) TestBootstrapConfig(c *gc.C) {
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"agent-version": "1.2.3",
	})
	c.Assert(inMap(attrs, "secret"), jc.IsTrue)
	c.Assert(inMap(attrs, "ca-private-key"), jc.IsTrue)
	c.Assert(inMap(attrs, "admin-secret"), jc.IsTrue)

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)

	cfg1, err := environs.BootstrapConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	expect := cfg.AllAttrs()
	expect["admin-secret"] = ""
	expect["ca-private-key"] = ""
	c.Assert(cfg1.AllAttrs(), gc.DeepEquals, expect)
}

type dummyProvider struct {
	environs.EnvironProvider
}

func (s *suite) TestRegisterProvider(c *gc.C) {
	s.PatchValue(environs.Providers, make(map[string]environs.EnvironProvider))
	s.PatchValue(environs.ProviderAliases, make(map[string]string))
	type step struct {
		name    string
		aliases []string
		err     string
	}
	type test []step

	tests := []test{
		[]step{{
			name: "providerName",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerName"},
			err:     "juju: duplicate provider alias \"providerName\"",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerAlias", "providerAlias"},
			err:     "juju: duplicate provider alias \"providerAlias\"",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerAlias1", "providerAlias2"},
		}},
		[]step{{
			name: "providerName",
		}, {
			name: "providerName",
			err:  "juju: duplicate provider name \"providerName\"",
		}},
		[]step{{
			name: "providerName1",
		}, {
			name:    "providerName2",
			aliases: []string{"providerName"},
		}},
		[]step{{
			name: "providerName1",
		}, {
			name:    "providerName2",
			aliases: []string{"providerName1"},
			err:     "juju: duplicate provider alias \"providerName1\"",
		}},
	}

	registerProvider := func(name string, aliases []string) (err error) {
		defer func() { err, _ = recover().(error) }()
		registered := &dummyProvider{}
		environs.RegisterProvider(name, registered, aliases...)
		p, err := environs.Provider(name)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p, gc.Equals, registered)
		for _, alias := range aliases {
			p, err := environs.Provider(alias)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(p, gc.Equals, registered)
			c.Assert(p, gc.Equals, registered)
		}
		return nil
	}
	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		for k := range *environs.Providers {
			delete(*environs.Providers, k)
		}
		for k := range *environs.ProviderAliases {
			delete(*environs.ProviderAliases, k)
		}
		for _, step := range test {
			err := registerProvider(step.name, step.aliases)
			if step.err == "" {
				c.Assert(err, jc.ErrorIsNil)
			} else {
				c.Assert(err, gc.ErrorMatches, step.err)
			}
		}
	}
}
