// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	_ "github.com/juju/juju/internal/provider/manual"
	"github.com/juju/juju/internal/testing"
)

type suite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&suite{})

type dummyProvider struct {
	environs.CloudEnvironProvider
}

func (s *suite) TestRegisterProvider(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(p, tc.Equals, registered)
		for _, alias := range aliases {
			p, err := environs.Provider(alias)
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(p, tc.Equals, registered)
			c.Assert(p, tc.Equals, registered)
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
				c.Assert(err, tc.ErrorIsNil)
			} else {
				c.Assert(err, tc.ErrorMatches, step.err)
			}
		}
	}
}

func (s *suite) TestUnregisterProvider(c *tc.C) {
	s.PatchValue(environs.Providers, make(map[string]environs.EnvironProvider))
	s.PatchValue(environs.ProviderAliases, make(map[string]string))
	registered := &dummyProvider{}
	unreg := environs.RegisterProvider("test", registered, "alias1", "alias2")
	unreg()
	_, err := environs.Provider("test")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	_, err = environs.Provider("alias1")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	_, err = environs.Provider("alias2")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}
