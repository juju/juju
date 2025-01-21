// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/juju/internal/packaging/manager"
)

// InstallMongo installs a mongo server for juju from snap using the specified
// snap channel.
func InstallMongo(snapChannel string) error {
	snapManager := manager.NewSnapPackageManager()
	return snapManager.Install(fmt.Sprintf("juju-db --channel %s", snapChannel))
}
