// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"os"

	"github.com/juju/juju/environs/gui"
)

const guiBaseURLEnvVar = "JUJU_GUI_SIMPLESTREAMS_URL"

// GUIDataSourceBaseURL returns the default base URL to use for the Juju GUI
// simplestreams data source. The default value can be overridden by setting
// the JUJU_GUI_SIMPLESTREAMS_URL environment variable.
func GUIDataSourceBaseURL() string {
	url := os.Getenv(guiBaseURLEnvVar)
	if url != "" {
		return url
	}
	return gui.DefaultBaseURL
}
