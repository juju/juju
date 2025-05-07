// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/version"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type baseSelectorSuite struct {
	testing.IsolationSuite

	logger logger.Logger
	cfg    *MockSelectorModelConfig
}

var _ = tc.Suite(&baseSelectorSuite{})

var (
	bionic      = base.MustParseBaseFromString("ubuntu@18.04/stable")
	cosmic      = base.MustParseBaseFromString("ubuntu@18.10/stable")
	disco       = base.MustParseBaseFromString("ubuntu@19.04")
	jammy       = base.MustParseBaseFromString("ubuntu@22.04")
	focal       = base.MustParseBaseFromString("ubuntu@20.04")
	precise     = base.MustParseBaseFromString("ubuntu@14.04")
	utopic      = base.MustParseBaseFromString("ubuntu@16.10")
	vivid       = base.MustParseBaseFromString("ubuntu@17.04")
	jujuDefault = version.DefaultSupportedLTSBase()
)

func (s *baseSelectorSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)

	s.cfg = NewMockSelectorModelConfig(ctrl)

	return ctrl
}

func (s *baseSelectorSuite) TestCharmBase(c *tc.C) {
	defer s.setup(c).Finish()

	deployBasesTests := []struct {
		title        string
		selector     BaseSelector
		expectedBase base.Base
		err          string
	}{
		{
			title: "juju deploy simple   # default base set",
			selector: BaseSelector{
				defaultBase:         focal,
				explicitDefaultBase: true,
				supportedBases:      []base.Base{bionic, focal},
			},
			expectedBase: focal,
		},
		{
			title: "juju deploy simple --base=ubuntu@14.04   # no supported base",
			selector: BaseSelector{
				requestedBase:  precise,
				supportedBases: []base.Base{bionic, cosmic},
			},
			err: `base: ubuntu@14.04/stable not supported`,
		},
		{
			title: "juju deploy simple --base=ubuntu@18.04   # user provided base takes precedence over default base ",
			selector: BaseSelector{
				requestedBase:       bionic,
				defaultBase:         focal,
				explicitDefaultBase: true,
				supportedBases:      []base.Base{bionic, focal, cosmic},
			},
			expectedBase: bionic,
		},

		// Now charms with supported base.

		{
			title: "juju deploy multiseries   # use charm default, nothing specified, no default base",
			selector: BaseSelector{
				supportedBases: []base.Base{bionic, cosmic},
			},
			expectedBase: bionic,
		},
		{
			title: "juju deploy multiseries   # use charm defaults used if default base doesn't match, nothing specified",
			selector: BaseSelector{
				supportedBases:      []base.Base{bionic, cosmic},
				defaultBase:         precise,
				explicitDefaultBase: true,
			},
			err: `base: ubuntu@14.04/stable not supported`,
		},
		{
			title: "juju deploy multiseries   # use model base defaults if supported by charm",
			selector: BaseSelector{
				supportedBases:      []base.Base{bionic, cosmic, disco},
				defaultBase:         disco,
				explicitDefaultBase: true,
			},
			expectedBase: disco,
		},
		{
			title: "juju deploy multiseries --base=ubuntu@18.04   # use supported requested",
			selector: BaseSelector{
				requestedBase:  bionic,
				supportedBases: []base.Base{cosmic, bionic},
			},
			expectedBase: bionic,
		},
		{
			title: "juju deploy multiseries --base=ubuntu@18.04   # unsupported requested",
			selector: BaseSelector{
				requestedBase:  bionic,
				supportedBases: []base.Base{utopic, vivid},
			},
			err: `base: ubuntu@18.04/stable not supported`,
		},
		{
			title: "juju deploy multiseries    # fallback to version.DefaultSupportedLTSBase()",
			selector: BaseSelector{
				supportedBases: []base.Base{utopic, vivid, jujuDefault},
			},
			expectedBase: jujuDefault,
		},
	}

	for i, test := range deployBasesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.selector.logger = s.logger
		base, err := test.selector.CharmBase()
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(base.IsCompatible(test.expectedBase), jc.IsTrue,
				tc.Commentf("%q compatible with %q", base, test.expectedBase))
		}
	}
}

func (s *baseSelectorSuite) TestValidate(c *tc.C) {
	defer s.setup(c).Finish()

	deploySeriesTests := []struct {
		title               string
		selector            BaseSelector
		supportedCharmBases []base.Base
		supportedJujuBases  []base.Base
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
		err: "base must be explicitly provided when image-id constraint is used forbidden",
	}, {
		title: "should return no errors when using image-id and base flag",
		selector: BaseSelector{
			requestedBase: jammy,
			usingImageID:  true,
		},
		supportedJujuBases:  []base.Base{jammy, bionic},
		supportedCharmBases: []base.Base{jammy, bionic},
	}, {
		title: "should return no errors when using image-id and explicit base from Config",
		selector: BaseSelector{
			explicitDefaultBase: true,
			usingImageID:        true,
		},
		supportedJujuBases:  []base.Base{jammy, bionic},
		supportedCharmBases: []base.Base{jammy, bionic},
	}, {
		title:               "juju deploy multiseries with invalid series  # use charm default, nothing specified, no default base",
		selector:            BaseSelector{},
		supportedCharmBases: []base.Base{precise},
		supportedJujuBases:  []base.Base{jammy},
		err:                 `the charm defined bases "ubuntu@14.04" not supported`,
	}}

	for i, test := range deploySeriesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.selector.logger = s.logger
		test.selector.jujuSupportedBases = set.NewStrings()
		_, err := test.selector.validate(test.supportedCharmBases, test.supportedJujuBases)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}

func (s *baseSelectorSuite) TestConfigureBaseSelector(c *tc.C) {
	defer s.setup(c).Finish()

	s.cfg.EXPECT().DefaultBase()
	cfg := SelectorConfig{
		Config:              s.cfg,
		Force:               false,
		Logger:              s.logger,
		RequestedBase:       base.Base{},
		SupportedCharmBases: []base.Base{jammy, focal, bionic},
		WorkloadBases:       []base.Base{jammy, focal},
		UsingImageID:        false,
	}

	obtained, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained.supportedBases, tc.DeepEquals, []base.Base{jammy, focal})
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorCentos(c *tc.C) {
	defer s.setup(c).Finish()

	s.cfg.EXPECT().DefaultBase()
	c7 := base.MustParseBaseFromString("centos@7/stable")
	c8 := base.MustParseBaseFromString("centos@8/stable")
	c6 := base.MustParseBaseFromString("centos@6/stable")
	cfg := SelectorConfig{
		Config:              s.cfg,
		Force:               false,
		Logger:              s.logger,
		RequestedBase:       base.Base{},
		SupportedCharmBases: []base.Base{c6, c7, c8},
		WorkloadBases:       []base.Base{c7, c8},
		UsingImageID:        false,
	}

	obtained, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained.supportedBases, tc.DeepEquals, []base.Base{c7, c8})
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorDefaultBase(c *tc.C) {
	defer s.setup(c).Finish()

	s.cfg.EXPECT().DefaultBase().Return("ubuntu@20.04", true)
	cfg := SelectorConfig{
		Config:              s.cfg,
		Force:               false,
		Logger:              s.logger,
		RequestedBase:       base.Base{},
		SupportedCharmBases: []base.Base{jammy, focal, bionic},
		WorkloadBases:       []base.Base{jammy, focal},
		UsingImageID:        false,
	}

	baseSelector, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(baseSelector.supportedBases, jc.SameContents, []base.Base{jammy, focal})
	c.Check(baseSelector.defaultBase, tc.DeepEquals, focal)
	c.Check(baseSelector.explicitDefaultBase, jc.IsTrue)

	obtained, err := baseSelector.CharmBase()
	c.Assert(err, jc.ErrorIsNil)
	expectedBase := base.MustParseBaseFromString("ubuntu@20.04")
	c.Check(obtained.IsCompatible(expectedBase), jc.IsTrue, tc.Commentf("obtained: %q, expected %q", obtained, expectedBase))
}

func (s *baseSelectorSuite) TestConfigureBaseSelectorDefaultBaseFail(c *tc.C) {
	defer s.setup(c).Finish()

	s.cfg.EXPECT().DefaultBase().Return("ubuntu@18.04", true)
	cfg := SelectorConfig{
		Config:              s.cfg,
		Force:               false,
		Logger:              s.logger,
		RequestedBase:       base.Base{},
		SupportedCharmBases: []base.Base{jammy, focal, bionic},
		WorkloadBases:       []base.Base{jammy, focal},
		UsingImageID:        false,
	}

	baseSelector, err := ConfigureBaseSelector(cfg)
	c.Assert(err, jc.ErrorIsNil)
	_, err = baseSelector.CharmBase()
	c.Assert(err, tc.ErrorMatches, `base: ubuntu@18.04/stable not supported`)
}
