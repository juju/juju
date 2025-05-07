// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/packaging/config"
)

var _ = tc.Suite(&AptSuite{})

type AptSuite struct {
	pacconfer config.AptConfigurer
}

func (s *AptSuite) SetUpSuite(c *tc.C) {
	s.pacconfer = config.NewAptPackagingConfigurer()
}

func (s *AptSuite) TestRenderPreferences(c *tc.C) {
	expected, err := testedPrefs.RenderPreferenceFile(config.AptPreferenceTemplate)
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.pacconfer.RenderPreferences(testedPrefs)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(res, tc.Equals, expected)
}
