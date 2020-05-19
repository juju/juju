// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	"github.com/juju/featureflag"

	"github.com/juju/juju/juju/osenv"
)

func init() {
	// If feature flags have been set on env, use them instead.
	if os.Getenv(osenv.JujuFeatureFlagEnvKey) != "" {
		featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	} else {
		featureflag.SetFlagsFromRegistry(osenv.JujuRegistryKey, osenv.JujuFeatureFlagEnvKey)
	}
}
