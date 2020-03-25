// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package main

import (
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/juju/osenv"
)

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}
