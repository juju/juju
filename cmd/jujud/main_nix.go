// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows
// +build !windows

package main

import (
	"os"

	"github.com/juju/featureflag"

	"github.com/juju/juju/internal/debug/coveruploader"
	"github.com/juju/juju/juju/osenv"
)

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func main() {
	coveruploader.Enable()
	MainWrapper(os.Args)
}
