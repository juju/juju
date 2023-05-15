// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/series"
	"github.com/juju/juju/version"
)

type baseSelectorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&baseSelectorSuite{})

var (
	bionic      = series.MustParseBaseFromString("ubuntu@18.04/stable")
	cosmic      = series.MustParseBaseFromString("ubuntu@18.10/stable")
	disco       = series.MustParseBaseFromString("ubuntu@19.04")
	jammy       = series.MustParseBaseFromString("ubuntu@22.04")
	focal       = series.MustParseBaseFromString("ubuntu@20.04")
	precise     = series.MustParseBaseFromString("ubuntu@14.04")
	utopic      = series.MustParseBaseFromString("ubuntu@16.10")
	vivid       = series.MustParseBaseFromString("ubuntu@17.04")
	latest      = series.LatestLTSBase()
	jujuDefault = version.DefaultSupportedLTSBase()
)

func (s *baseSelectorSuite) TestCharmBase(c *gc.C) {
	deployBasesTests := []struct {
		title        string
		selector     BaseSelector
		expectedBase series.Base
		err          string
	}{
		{
			title: "juju deploy simple   # default base set",
			selector: BaseSelector{
				defaultBase:         focal,
				explicitDefaultBase: true,
				supportedBases:      []series.Base{bionic, focal},
			},
			expectedBase: focal,
		},
		{
			title: "juju deploy simple --base=ubuntu@14.04   # no supported base",
			selector: BaseSelector{
				requestedBase:  precise,
				supportedBases: []series.Base{bionic, cosmic},
			},
			err: `base "ubuntu@14.04" not supported by charm, the charm supported bases are: ubuntu@18.04, ubuntu@18.10`,
		},
		{
			title: "juju deploy simple --base=ubuntu@18.04   # user provided base takes precedence over default base ",
			selector: BaseSelector{
				requestedBase:       bionic,
				defaultBase:         focal,
				explicitDefaultBase: true,
				supportedBases:      []series.Base{bionic, focal, cosmic},
			},
			expectedBase: bionic,
		},

		// Now charms with supported base.

		{
			title: "juju deploy multiseries   # use charm default, nothing specified, no default base",
			selector: BaseSelector{
				supportedBases: []series.Base{bionic, cosmic},
			},
			expectedBase: bionic,
		},
		{
			title: "juju deploy multiseries   # use charm defaults used if default base doesn't match, nothing specified",
			selector: BaseSelector{
				supportedBases:      []series.Base{bionic, cosmic},
				defaultBase:         precise,
				explicitDefaultBase: true,
			},
			err: `base "ubuntu@14.04" not supported by charm, the charm supported bases are: ubuntu@18.04, ubuntu@18.10`,
		},
		{
			title: "juju deploy multiseries   # use model base defaults if supported by charm",
			selector: BaseSelector{
				supportedBases:      []series.Base{bionic, cosmic, disco},
				defaultBase:         disco,
				explicitDefaultBase: true,
			},
			expectedBase: disco,
		},
		{
			title: "juju deploy multiseries --base=ubuntu@18.04   # use supported requested",
			selector: BaseSelector{
				requestedBase:  bionic,
				supportedBases: []series.Base{cosmic, bionic},
			},
			expectedBase: bionic,
		},
		{
			title: "juju deploy multiseries --base=ubuntu@18.04   # unsupported requested",
			selector: BaseSelector{
				requestedBase:  bionic,
				supportedBases: []series.Base{utopic, vivid},
			},
			err: `base "ubuntu@18.04" not supported by charm, the charm supported bases are: ubuntu@16.10, ubuntu@17.04`,
		},
		{
			title: "juju deploy multiseries    # fallback to series.LatestLTSBase()",
			selector: BaseSelector{
				supportedBases: []series.Base{utopic, vivid, latest},
			},
			expectedBase: latest,
		},
		{
			title: "juju deploy multiseries    # fallback to version.DefaultSupportedLTSBase()",
			selector: BaseSelector{
				supportedBases: []series.Base{utopic, vivid, jujuDefault},
			},
			expectedBase: jujuDefault,
		},
		{
			title: "juju deploy multiseries    # prefer series.LatestLTSBase() to  version.DefaultSupportedLTSBase()",
			selector: BaseSelector{
				supportedBases: []series.Base{utopic, vivid, jujuDefault, latest},
			},
			expectedBase: latest,
		},
	}

	// Use bionic for LTS for all calls.
	previous := series.SetLatestLtsForTesting("bionic")
	defer series.SetLatestLtsForTesting(previous)

	for i, test := range deployBasesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.selector.logger = &noOpLogger{}
		// For test purposes, change the supportedJujuBases to a consistent
		// known value. Allowing for juju supported bases to change without
		// making the tests fail.
		//baseSelect.supportedJujuBases = []series.Base{bionic, focal, cosmic}
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
		supportedCharmBases []series.Base
		supportedJujuBases  []series.Base
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
		supportedJujuBases:  []series.Base{jammy, bionic},
		supportedCharmBases: []series.Base{jammy, bionic},
	}, {
		title: "should return no errors when using image-id and explicit base from Config",
		selector: BaseSelector{
			explicitDefaultBase: true,
			usingImageID:        true,
		},
		supportedJujuBases:  []series.Base{jammy, bionic},
		supportedCharmBases: []series.Base{jammy, bionic},
	}, {
		title:               "juju deploy multiseries with invalid series  # use charm default, nothing specified, no default base",
		selector:            BaseSelector{},
		supportedCharmBases: []series.Base{precise},
		supportedJujuBases:  []series.Base{jammy},
		err:                 `the charm defined bases "ubuntu@14.04" not supported`,
	}}

	for i, test := range deploySeriesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.selector.logger = &noOpLogger{}
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
		RequestedBase:       series.Base{},
		SupportedCharmBases: []series.Base{jammy, focal, bionic},
		UsingImageID:        false,
	}

	obtained, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained.supportedBases, gc.DeepEquals, []series.Base{jammy, focal})
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorCentos(c *gc.C) {

	c7 := series.MustParseBaseFromString("centos@7/stable")
	c8 := series.MustParseBaseFromString("centos@8/stable")
	c6 := series.MustParseBaseFromString("centos@6/stable")
	cfg := SelectorConfig{
		Config:              mockModelCfg{},
		Force:               false,
		Logger:              &noOpLogger{},
		RequestedBase:       series.Base{},
		SupportedCharmBases: []series.Base{c6, c7, c8},
		UsingImageID:        false,
	}

	obtained, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained.supportedBases, gc.DeepEquals, []series.Base{c7, c8})
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorDefaultBase(c *gc.C) {
	cfg := SelectorConfig{
		Config: mockModelCfg{
			base:     "ubuntu@20.04",
			explicit: true,
		},
		Force:               false,
		Logger:              &noOpLogger{},
		RequestedBase:       series.Base{},
		SupportedCharmBases: []series.Base{jammy, focal, bionic},
		UsingImageID:        false,
	}

	obtained, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained.supportedBases, jc.SameContents, []series.Base{jammy, focal})
	c.Check(obtained.defaultBase, gc.DeepEquals, focal)
	c.Check(obtained.explicitDefaultBase, jc.IsTrue)
}
