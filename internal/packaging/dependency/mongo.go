// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/juju/internal/packaging/manager"
)

// InstallMongo installs a mongo server for juju from snap using the specified
// snap channel.
func InstallMongo() error {
	snapManager := manager.NewSnapPackageManager()
	return snapManager.Install("juju-db --channel 4.4/stable")
}
