// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/config"
)

var _ = gc.Suite(&AptSuite{})

type AptSuite struct {
	pacconfer config.AptConfigurer
}

func (s *AptSuite) SetUpSuite(c *gc.C) {
	s.pacconfer = config.NewAptPackagingConfigurer()
}

func (s *AptSuite) TestRenderPreferences(c *gc.C) {
	expected, err := testedPrefs.RenderPreferenceFile(config.AptPreferenceTemplate)
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.pacconfer.RenderPreferences(testedPrefs)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(res, gc.Equals, expected)
}
