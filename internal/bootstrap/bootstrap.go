// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"os"
	"path/filepath"

	"github.com/juju/juju/internal/cloudconfig"
)

// BootstrapParamsPath returns the path to the bootstrap params file.
func BootstrapParamsPath(dataDir string) string {
	return filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams)
}

// IsBootstrapController returns whether the controller is a bootstrap
// controller.
func IsBootstrapController(dataDir string) bool {
	_, err := os.Stat(BootstrapParamsPath(dataDir))
	return !os.IsNotExist(err)
}
