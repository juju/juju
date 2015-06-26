// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/utils/packaging"
	"github.com/juju/utils/proxy"
)

// windowsCloudConfig is the cloudconfig type specific to Windows machines.
// It mostly deals entirely with passing the equivalent of runcmds to
// cloudbase-init, leaving most of the other functionalities uninmplemented.
// It implements the CloudConfig interface.
type windowsCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *windowsCloudConfig) SetPackageProxy(url string) {
}

// UnsetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *windowsCloudConfig) UnsetPackageProxy() {
}

// PackageProxy is defined on the PackageProxyConfig interface.
func (cfg *windowsCloudConfig) PackageProxy() string {
	return ""
}

// SetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *windowsCloudConfig) SetPackageMirror(url string) {
}

// UnsetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *windowsCloudConfig) UnsetPackageMirror() {
}

// PackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *windowsCloudConfig) PackageMirror() string {
	return ""
}

// AddPackageSource is defined on the PackageSourcesConfig interface.
func (cfg *windowsCloudConfig) AddPackageSource(src packaging.PackageSource) {
}

// PackageSources is defined on the PackageSourcesConfig interface.
func (cfg *windowsCloudConfig) PackageSources() []packaging.PackageSource {
	return nil
}

// AddPackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *windowsCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
}

// PackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *windowsCloudConfig) PackagePreferences() []packaging.PackagePreferences {
	return nil
}

// RenderYAML is defined on the RenderConfig interface.
func (cfg *windowsCloudConfig) RenderYAML() ([]byte, error) {
	return cfg.renderWindows()
}

// RenderScript is defined on the RenderConfig interface.
func (cfg *windowsCloudConfig) RenderScript() (string, error) {
	// NOTE: This shouldn't really be called on windows as it's used only for
	// initialization via ssh or on local providers.
	script, err := cfg.renderWindows()
	if err != nil {
		return "", err
	}

	return string(script), err
}

// getCommandsForAddingPackages is defined on the RenderConfig interface.
func (cfg *windowsCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	return nil, nil
}

// renderWindows is a helper function which renders the runCmds of the Windows
// CloudConfig to a PowerShell script.
func (cfg *windowsCloudConfig) renderWindows() ([]byte, error) {
	winCmds := cfg.RunCmds()
	var script []byte
	newline := "\r\n"
	header := "#ps1_sysnative\r\n"
	script = append(script, header...)
	for _, cmd := range winCmds {
		script = append(script, newline...)
		script = append(script, cmd...)
	}
	return script, nil
}

// AddPackageCommands is defined on the AdvancedPackagingConfig interface.
func (cfg *windowsCloudConfig) AddPackageCommands(
	aptProxySettings proxy.Settings,
	aptMirror string,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) {
	return
}

// AddCloudArchiveCloudTools is defined on the AdvancedPackagingConfig
// interface.
func (cfg *windowsCloudConfig) AddCloudArchiveCloudTools() {
}

// addRequiredPackages is defined on the AdvancedPackagingConfig interface.
func (cfg *windowsCloudConfig) addRequiredPackages() {
}

// updateProxySettings is defined on the AdvancedPackagingConfig interface.
func (cfg *windowsCloudConfig) updateProxySettings(proxy.Settings) {
}
