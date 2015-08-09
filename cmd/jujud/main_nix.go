// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package main

import (
	"os"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/utils/featureflag"
)

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func main() {
	MainWrapper(os.Args)
}
