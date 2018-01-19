// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/manual"
	"github.com/juju/juju/testing"
)

type suite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.AddCleanup(dummy.Reset)
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

type dummyProvider struct {
	environs.EnvironProvider
}

func (s *suite) TestRegisterProvider(c *gc.C) {
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
			err:     "duplicate provider alias \"providerName\"",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerAlias", "providerAlias"},
			err:     "duplicate provider alias \"providerAlias\"",
		}},
		[]step{{
			name:    "providerName",
			aliases: []string{"providerAlias1", "providerAlias2"},
		}},
		[]step{{
			name: "providerName",
		}, {
			name: "providerName",
			err:  "duplicate provider name \"providerName\"",
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
			err:     "duplicate provider alias \"providerName1\"",
		}},
	}

	var registry *environs.ProviderRegistry
	registerProvider := func(name string, aliases []string) error {
		registered := &dummyProvider{}
		err := registry.Register(registered, name, aliases...)
		if err != nil {
			return err
		}
		p, err := registry.Provider(name)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p, gc.Equals, registered)
		for _, alias := range aliases {
			p, err := registry.Provider(alias)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(p, gc.Equals, registered)
			c.Assert(p, gc.Equals, registered)
		}
		return nil
	}
	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		registry = environs.NewProviderRegistry()
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

func (s *suite) TestUnregisterProvider(c *gc.C) {
	registry := environs.NewProviderRegistry()
	registered := &dummyProvider{}
	err := registry.Register(registered, "test", "alias1", "alias2")
	c.Assert(err, jc.ErrorIsNil)
	registry.Unregister("test")
	_, err = registry.Provider("test")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = registry.Provider("alias1")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = registry.Provider("alias2")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
