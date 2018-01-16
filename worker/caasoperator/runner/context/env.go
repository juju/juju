// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"

	jujuos "github.com/juju/utils/os"
)

// OSEnvVars returns the OS environment variables that
// should be set for a hook context.
func OSEnvVars(paths Paths) []string {
	switch jujuos.HostOS() {
	case jujuos.Ubuntu, jujuos.CentOS, jujuos.OpenSUSE:
		return appendPath(paths)
	}
	return nil
}

func appendPath(paths Paths) []string {
	return []string{
		"PATH=" + paths.GetToolsDir() + ":" + os.Getenv("PATH"),
	}
}
