// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/juju/osenv"
)

// FLAGSFROMENVIRONMENT can control whether we read featureflags from the
// environment or from the registry. This is only needed because we build the
// jujud binary in uniter tests and we cannot mock the registry out easily.
// Once uniter tests are fixed this should be removed.
var FLAGSFROMENVIRONMENT string

func init() {
	if FLAGSFROMENVIRONMENT == "true" {
		featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	} else {
		featureflag.SetFlagsFromRegistry(osenv.JujuRegistryKey, osenv.JujuFeatureFlagEnvKey)
	}
}
