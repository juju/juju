// Copyright 2012, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package testing

import (
	"os"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/utils/featureflag"
)

// setUpFeatureFlags sets the feature flags from the environment.
func (s *JujuOSEnvSuite) setUpFeatureFlags(c *gc.C) {
	os.Setenv(osenv.JujuFeatureFlagEnvKey, s.initialFeatureFlags)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

// Feature flags are restored when restoring the old env values
// which happens on both platforms
func (s *JujuOSEnvSuite) tearDownFeatureFlags(c *gc.C) {
}
