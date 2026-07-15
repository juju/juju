// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"os"
	"path/filepath"

	"github.com/juju/juju/internal/cloudconfig"
)

// BootstrapParamsPath returns the path to the bootstrap params file.
//
// If the environment variable JUJU_BOOTSTRAP_PARAMS_PATH is set, that value is
// returned directly, allowing snap-managed controllers to keep the bootstrap
// params file in a snap-common location separate from DataDir.
//
// For snap IAAS controllers the bootstrap params are staged to
// $SNAP_COMMON/bootstrap-params by jujud init and the daemon app reads them
// through the JUJU_BOOTSTRAP_PARAMS_PATH environment variable set in
// snapcraft.yaml. The machine agent (jujuagentd) on the same host does not
// receive a copy of bootstrap-params; its host DataDir path is irrelevant for
// the snap controller's initialization.
func BootstrapParamsPath(dataDir string) string {
	if path := os.Getenv("JUJU_BOOTSTRAP_PARAMS_PATH"); path != "" {
		return path
	}
	return filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams)
}

// IsBootstrapController returns whether the controller is a bootstrap
// controller.
func IsBootstrapController(dataDir string) bool {
	_, err := os.Stat(BootstrapParamsPath(dataDir))
	return !os.IsNotExist(err)
}
