// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"github.com/juju/juju/version"
	"github.com/juju/utils/packaging/manager"
)

// getPackageManager is a helper function which returns the
// package manager implementation for the current system.
func getPackageManager() (manager.PackageManager, error) {
	return manager.NewPackageManager(version.Current.Series)
}
