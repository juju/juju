// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"fmt"
	"strings"

	"github.com/juju/utils/packaging/commands"
	"github.com/juju/utils/proxy"
)

// basePackageManager is the struct which executes various
// packaging-related operations.
type basePackageManager struct {
	cmder commands.PackageCommander
}

// InstallPrerequisite is defined on the PackageManager interface.
func (pm *basePackageManager) InstallPrerequisite() error {
	_, _, err := RunCommandWithRetry(pm.cmder.InstallPrerequisiteCmd(), nil)
	return err
}

// Update is defined on the PackageManager interface.
func (pm *basePackageManager) Update() error {
	_, _, err := RunCommandWithRetry(pm.cmder.UpdateCmd(), nil)
	return err
}

// Upgrade is defined on the PackageManager interface.
func (pm *basePackageManager) Upgrade() error {
	_, _, err := RunCommandWithRetry(pm.cmder.UpgradeCmd(), nil)
	return err
}

// Install is defined on the PackageManager interface.
func (pm *basePackageManager) Install(packs ...string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.InstallCmd(packs...), nil)
	return err
}

// Remove is defined on the PackageManager interface.
func (pm *basePackageManager) Remove(packs ...string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.RemoveCmd(packs...), nil)
	return err
}

// Purge is defined on the PackageManager interface.
func (pm *basePackageManager) Purge(packs ...string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.PurgeCmd(packs...), nil)
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
	_, _, err := RunCommandWithRetry(pm.cmder.AddRepositoryCmd(repo), nil)
	return err
}

// RemoveRepository is defined on the PackageManager interface.
func (pm *basePackageManager) RemoveRepository(repo string) error {
	_, _, err := RunCommandWithRetry(pm.cmder.RemoveRepositoryCmd(repo), nil)
	return err
}

// Cleanup is defined on the PackageManager interface.
func (pm *basePackageManager) Cleanup() error {
	_, _, err := RunCommandWithRetry(pm.cmder.CleanupCmd(), nil)
	return err
}

// SetProxy is defined on the PackageManager interface.
func (pm *basePackageManager) SetProxy(settings proxy.Settings) error {
	cmds := pm.cmder.SetProxyCmds(settings)

	for _, cmd := range cmds {
		args := []string{"bash", "-c", fmt.Sprintf("%q", cmd)}
		out, err := RunCommand(args[0], args[1:]...)
		if err != nil {
			logger.Errorf("command failed: %v\nargs: %#v\n%s", err, args, string(out))
			return fmt.Errorf("command failed: %v", err)
		}
	}

	return nil
}
