// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/utils/packaging/manager"

	"github.com/juju/juju/version"
)

// getPackageManager is a helper function which returns the
// package manager implementation for the current system.
func getPackageManager() (manager.PackageManager, error) {
	return manager.NewPackageManager(version.Current.Series)
}

// installRsyslogGnutls installs the rsyslog-gnutls package,
// which is required for our rsyslog configuration from 1.18.0.
func installRsyslogGnutls(context Context) error {
	pacman, err := getPackageManager()
	if err != nil {
		return err
	}

	return pacman.Install("rsyslog-gnutls")
}
