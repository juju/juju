// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/version"
)

type baseSelectorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&baseSelectorSuite{})

var (
	bionic      = corebase.MustParseBaseFromString("ubuntu@18.04/stable")
	cosmic      = corebase.MustParseBaseFromString("ubuntu@18.10/stable")
	disco       = corebase.MustParseBaseFromString("ubuntu@19.04")
	jammy       = corebase.MustParseBaseFromString("ubuntu@22.04")
	focal       = corebase.MustParseBaseFromString("ubuntu@20.04")
	precise     = corebase.MustParseBaseFromString("ubuntu@14.04")
	utopic      = corebase.MustParseBaseFromString("ubuntu@16.10")
	vivid       = corebase.MustParseBaseFromString("ubuntu@17.04")
	latest      = corebase.LatestLTSBase()
	jujuDefault = version.DefaultSupportedLTSBase()
)

func (s *baseSelectorSuite) TestCharmBase(c *gc.C) {
	deployBasesTests := []struct {
		title        string
		selector     BaseSelector
		expectedBase corebase.Base
		err          string
	}{
		{
			title: "juju deploy simple   # default base set",
			selector: BaseSelector{
				defaultBase:         focal,
				explicitDefaultBase: true,
				supportedBases:      []corebase.Base{bionic, focal},
			},
			expectedBase: focal,
		},
		{
			title: "juju deploy simple --base=ubuntu@14.04   # no supported base",
			selector: BaseSelector{
				requestedBase:  precise,
				supportedBases: []corebase.Base{bionic, cosmic},
			},
			err: `base: ubuntu@14.04/stable`,
		},
		{
			title: "juju deploy simple --base=ubuntu@18.04   # user provided base takes precedence over default base ",
			selector: BaseSelector{
				requestedBase:       bionic,
				defaultBase:         focal,
				explicitDefaultBase: true,
				supportedBases:      []corebase.Base{bionic, focal, cosmic},
			},
			expectedBase: bionic,
		},

		// Now charms with supported base.

		{
			title: "juju deploy multicorebase   # use charm default, nothing specified, no default base",
			selector: BaseSelector{
				supportedBases: []corebase.Base{bionic, cosmic},
			},
			expectedBase: bionic,
		},
		{
			title: "juju deploy multicorebase   # use charm defaults used if default base doesn't match, nothing specified",
			selector: BaseSelector{
				supportedBases:      []corebase.Base{bionic, cosmic},
				defaultBase:         precise,
				explicitDefaultBase: true,
			},
			err: `base: ubuntu@14.04/stable`,
		},
		{
			title: "juju deploy multiseries   # use model base defaults if supported by charm",
			selector: BaseSelector{
				supportedBases:      []corebase.Base{bionic, cosmic, disco},
				defaultBase:         disco,
				explicitDefaultBase: true,
			},
			expectedBase: disco,
		},
		{
			title: "juju deploy multiseries --base=ubuntu@18.04   # use supported requested",
			selector: BaseSelector{
				requestedBase:  bionic,
				supportedBases: []corebase.Base{cosmic, bionic},
			},
			expectedBase: bionic,
		},
		{
			title: "juju deploy multiseries --base=ubuntu@18.04   # unsupported requested",
			selector: BaseSelector{
				requestedBase:  bionic,
				supportedBases: []corebase.Base{utopic, vivid},
			},
			err: `base: ubuntu@18.04/stable`,
		},
		{
			title: "juju deploy multiseries    # fallback to corebase.LatestLTSBase()",
			selector: BaseSelector{
				supportedBases: []corebase.Base{utopic, vivid, latest},
			},
			expectedBase: latest,
		},
		{
			title: "juju deploy multiseries    # fallback to version.DefaultSupportedLTSBase()",
			selector: BaseSelector{
				supportedBases: []corebase.Base{utopic, vivid, jujuDefault},
			},
			expectedBase: jujuDefault,
		},
		{
			title: "juju deploy multiseries    # prefer corebase.LatestLTSBase() to  version.DefaultSupportedLTSBase()",
			selector: BaseSelector{
				supportedBases: []corebase.Base{utopic, vivid, jujuDefault, latest},
			},
			expectedBase: latest,
		},
	}

	// Use bionic for LTS for all calls.
	previous := corebase.SetLatestLtsForTesting("focal")
	defer corebase.SetLatestLtsForTesting(previous)

	for i, test := range deployBasesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.selector.logger = &noOpLogger{}
		// For test purposes, change the supportedJujuBases to a consistent
		// known value. Allowing for juju supported bases to change without
		// making the tests fail.
		//baseSelect.supportedJujuBases = []corebase.Base{bionic, focal, cosmic}
		base, err := test.selector.CharmBase()
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(base.IsCompatible(test.expectedBase), jc.IsTrue,
				gc.Commentf("%q compatible with %q", base, test.expectedBase))
		}
	}
}

func (s *baseSelectorSuite) TestValidate(c *gc.C) {
	deploySeriesTests := []struct {
		title               string
		selector            BaseSelector
		supportedCharmBases []corebase.Base
		supportedJujuBases  []corebase.Base
		err                 string
	}{{
		// Simple selectors first, no supported bases, check we're validating
		title:    "juju deploy simple   # no default base, no supported base",
		selector: BaseSelector{},
		err:      "charm does not define any bases, not valid",
	}, {
		title: "should fail when image-id constraint is used and no base is explicitly set",
		selector: BaseSelector{
			usingImageID: true,
		},
		err: "base must be explicitly provided when image-id constraint is used",
	}, {
		title: "should return no errors when using image-id and base flag",
		selector: BaseSelector{
			requestedBase: jammy,
			usingImageID:  true,
		},
		supportedJujuBases:  []corebase.Base{jammy, bionic},
		supportedCharmBases: []corebase.Base{jammy, bionic},
	}, {
		title: "should return no errors when using image-id and explicit base from Config",
		selector: BaseSelector{
			explicitDefaultBase: true,
			usingImageID:        true,
		},
		supportedJujuBases:  []corebase.Base{jammy, bionic},
		supportedCharmBases: []corebase.Base{jammy, bionic},
	}, {
		title:               "juju deploy multiseries with invalid series  # use charm default, nothing specified, no default base",
		selector:            BaseSelector{},
		supportedCharmBases: []corebase.Base{precise},
		supportedJujuBases:  []corebase.Base{jammy},
		err:                 `the charm defined bases "ubuntu@14.04" not supported`,
	}}

	for i, test := range deploySeriesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.selector.logger = &noOpLogger{}
		test.selector.jujuSupportedBases = set.NewStrings()
		_, err := test.selector.validate(test.supportedCharmBases, test.supportedJujuBases)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}

func (s *baseSelectorSuite) TestConfigureBaseSelector(c *gc.C) {
	cfg := SelectorConfig{
		Config:              mockModelCfg{},
		Force:               false,
		Logger:              &noOpLogger{},
		RequestedBase:       corebase.Base{},
		SupportedCharmBases: []corebase.Base{jammy, focal, bionic},
		WorkloadBases:       []corebase.Base{jammy, focal},
		UsingImageID:        false,
	}

	obtained, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained.supportedBases, gc.DeepEquals, []corebase.Base{jammy, focal})
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorCentos(c *gc.C) {

	c7 := corebase.MustParseBaseFromString("centos@7/stable")
	c8 := corebase.MustParseBaseFromString("centos@8/stable")
	c6 := corebase.MustParseBaseFromString("centos@6/stable")
	cfg := SelectorConfig{
		Config:              mockModelCfg{},
		Force:               false,
		Logger:              &noOpLogger{},
		RequestedBase:       corebase.Base{},
		SupportedCharmBases: []corebase.Base{c6, c7, c8},
		WorkloadBases:       []corebase.Base{c7, c8},
		UsingImageID:        false,
	}

	obtained, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained.supportedBases, gc.DeepEquals, []corebase.Base{c7, c8})
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorDefaultBase(c *gc.C) {
	cfg := SelectorConfig{
		Config: mockModelCfg{
			base:     "ubuntu@20.04",
			explicit: true,
		},
		Force:               false,
		Logger:              &noOpLogger{},
		RequestedBase:       corebase.Base{},
		SupportedCharmBases: []corebase.Base{jammy, focal, bionic},
		WorkloadBases:       []corebase.Base{jammy, focal},
		UsingImageID:        false,
	}

	baseSelector, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(baseSelector.supportedBases, jc.SameContents, []corebase.Base{jammy, focal})
	c.Check(baseSelector.defaultBase, gc.DeepEquals, focal)
	c.Check(baseSelector.explicitDefaultBase, jc.IsTrue)

	obtained, err := baseSelector.CharmBase()
	c.Assert(err, jc.ErrorIsNil)
	expectedBase := corebase.MustParseBaseFromString("ubuntu@20.04")
	c.Check(obtained.IsCompatible(expectedBase), jc.IsTrue, gc.Commentf("obtained: %q, expected %q", obtained, expectedBase))
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorDefaultBaseFail(c *gc.C) {
	cfg := SelectorConfig{
		Config: mockModelCfg{
			base:     "ubuntu@18.04",
			explicit: true,
		},
		Force:               false,
		Logger:              &noOpLogger{},
		RequestedBase:       corebase.Base{},
		SupportedCharmBases: []corebase.Base{jammy, focal, bionic},
		WorkloadBases:       []corebase.Base{jammy, focal},
		UsingImageID:        false,
	}

	baseSelector, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	_, err = baseSelector.CharmBase()
	c.Assert(err, gc.ErrorMatches, `base: ubuntu@18.04/stable`)
}
