// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"fmt"
	"strings"

	"github.com/juju/proxy"

	"github.com/juju/juju/internal/packaging/commands"
)

// basePackageManager is the struct which executes various
// packaging-related operations.
type basePackageManager struct {
	cmder       commands.PackageCommander
	retryable   Retryable
	retryPolicy RetryPolicy
}

// InstallPrerequisite is defined on the PackageManager interface.
func (pm *basePackageManager) InstallPrerequisite() error {
	_, _, err := RunCommandWithRetry(pm.cmder.InstallPrerequisiteCmd(), pm, pm.retryPolicy)
	return err
}

// Update is defined on the PackageManager interface.
func (pm *basePackageManager) Update() error {
	_, _, err := RunCommandWithRetry(pm.cmder.UpdateCmd(), pm, pm.retryPolicy)
	return err
}

// Upgrade is defined on the PackageManager interface.
func (pm *basePackageManager) Upgrade() error {
	_, _, err := RunCommandWithRetry(pm.cmder.UpgradeCmd(), pm, pm.retryPolicy)
	return err
}

// Install is defined on the PackageManager interface.
func (pm *basePackageManager) Install(packs ...string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.InstallCmd(packs...), pm, pm.retryPolicy)
	return err
}

// Remove is defined on the PackageManager interface.
func (pm *basePackageManager) Remove(packs ...string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.RemoveCmd(packs...), pm, pm.retryPolicy)
	return err
}

// Purge is defined on the PackageManager interface.
func (pm *basePackageManager) Purge(packs ...string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.PurgeCmd(packs...), pm, pm.retryPolicy)
	return err
}

// IsInstalled is defined on the PackageManager interface.
func (pm *basePackageManager) IsInstalled(pack string) bool {
	args := strings.Fields(pm.cmder.IsInstalledCmd(pack))

	_, err := RunCommand(args[0], args[1:]...)
	return err == nil
}

// AddRepository is defined on the PackageManager interface.
func (pm *basePackageManager) AddRepository(repo string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.AddRepositoryCmd(repo), pm, pm.retryPolicy)
	return err
}

// RemoveRepository is defined on the PackageManager interface.
func (pm *basePackageManager) RemoveRepository(repo string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.RemoveRepositoryCmd(repo), pm, pm.retryPolicy)
	return err
}

// Cleanup is defined on the PackageManager interface.
func (pm *basePackageManager) Cleanup() error {
	_, _, err := RunCommandWithRetry(pm.cmder.CleanupCmd(), pm, pm.retryPolicy)
	return err
}

// SetProxy is defined on the PackageManager interface.
func (pm *basePackageManager) SetProxy(settings proxy.Settings) error {
	for _, cmd := range pm.cmder.SetProxyCmds(settings) {
		args := []string{"bash", "-c", fmt.Sprintf("%q", cmd)}
		out, err := RunCommand(args[0], args[1:]...)
		if err != nil {
			logger.Errorf("command failed: %v\nargs: %#v\n%s", err, args, out)
			return fmt.Errorf("command failed: %v", err)
		}
	}

	return nil
}

func (pm *basePackageManager) IsRetryable(code int, output string) bool {
	if pm.retryable != nil {
		return pm.retryable.IsRetryable(code, output)
	}
	return false
}
