// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"github.com/juju/juju/internal/packaging/commands"
)

// basePackageManager is the struct which executes various
// packaging-related operations.
type basePackageManager struct {
	cmder       commands.PackageCommander
	retryable   Retryable
	retryPolicy RetryPolicy
}

// Install is defined on the PackageManager interface.
func (pm *basePackageManager) Install(packs ...string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.InstallCmd(packs...), pm, pm.retryPolicy)
	return err
}

func (pm *basePackageManager) IsRetryable(code int, output string) bool {
	if pm.retryable != nil {
		return pm.retryable.IsRetryable(code, output)
	}
	return false
}
