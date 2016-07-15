// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// +build !windows

package featureflag

import (
	"os"
)

// getFlagsFromRegistry should theoretically never get called on linux, but even if it does
// this does the right thing which is using the environment.
func getFlagsFromRegistry(envVarKey, envVarName string) string {
	return os.Getenv(envVarName)
}
