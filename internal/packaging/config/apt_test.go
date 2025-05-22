// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/packaging/config"
)

func TestAptSuite(t *testing.T) {
	tc.Run(t, &AptSuite{})
}

type AptSuite struct {
	pacconfer config.AptConfigurer
}

func (s *AptSuite) SetUpSuite(c *tc.C) {
	s.pacconfer = config.NewAptPackagingConfigurer()
}

func (s *AptSuite) TestRenderPreferences(c *tc.C) {
	expected, err := testedPrefs.RenderPreferenceFile(config.AptPreferenceTemplate)
	c.Assert(err, tc.ErrorIsNil)

	res, err := s.pacconfer.RenderPreferences(testedPrefs)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res, tc.Equals, expected)
}
