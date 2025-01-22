// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/juju/internal/packaging/manager"
)

// InstallLXD installs the LXD snap using the specified snap channel.
func InstallLXD(snapChannel string) error {
	snapManager := manager.NewSnapPackageManager()
	return snapManager.Install(fmt.Sprintf("--classic --channel %s lxd", snapChannel))
}
