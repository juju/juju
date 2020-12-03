// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"os"

	"github.com/juju/juju/environs/gui"
)

const dashboartdBaseURLEnvVar = "JUJU_DASHBOARD_SIMPLESTREAMS_URL"

// DashboardDataSourceBaseURL returns the default base URL to use for the Juju Dashboard
// simplestreams data source. The default value can be overridden by setting
// the JUJU_DASHBOARD_SIMPLESTREAMS_URL environment variable.
func DashboardDataSourceBaseURL() string {
	url := os.Getenv(dashboartdBaseURLEnvVar)
	if url != "" {
		return url
	}
	return gui.DefaultBaseURL
}
