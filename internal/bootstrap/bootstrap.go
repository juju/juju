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
// TODO(juju-10104) The JUJU_BOOTSTRAP_PARAMS_PATH override exists for the
// transitional dual-copy setup in Phase 1, where both the controller snap
// ($SNAP_COMMON/bootstrap-params) and jujuagentd (/var/lib/juju/bootstrap-params)
// need their own copy. Once Stage 5 removes controller manifolds from
// jujuagentd, the dual-copy model and this env-var override can be removed.
// At that point BootstrapParamsPath should derive directly from DataDir.
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
