// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file which is Windows compatible.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"github.com/juju/utils/packaging"
	"github.com/juju/utils/proxy"
)

// WindowsCloudConfig is the cloudconfig type specific to Ubuntu machines
// It simply contains a cloudConfig with the added package management-related
// methods for the Ubuntu version of cloudinit.
// It satisfies the cloudinit.CloudConfig interface.
type WindowsCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *WindowsCloudConfig) SetPackageProxy(url string) {
	return
}

// UnsetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *WindowsCloudConfig) UnsetPackageProxy() {
	return
}

// PackageProxy is defined on the PackageProxyConfig interface.
func (cfg *WindowsCloudConfig) PackageProxy() string {
	return ""
}

// SetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *WindowsCloudConfig) SetPackageMirror(url string) {
	return
}

// UnsetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *WindowsCloudConfig) UnsetPackageMirror() {
	return
}

// PackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *WindowsCloudConfig) PackageMirror() string {
	return ""
}

// AddPackageSource is defined on the PackageSourcesConfig interface.
func (cfg *WindowsCloudConfig) AddPackageSource(src packaging.PackageSource) {
	return
}

// PackageSources is defined on the PackageSourcesConfig interface.
func (cfg *WindowsCloudConfig) PackageSources() []packaging.PackageSource {
	// NOTE: this should not ever get called, so it is safe to return nil here:
	return nil
}

// AddPackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *WindowsCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	return
}

// PackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *WindowsCloudConfig) PackagePreferences() []packaging.PackagePreferences {
	// NOTE: this should not ever get called, so it is safe to return nil here:
	return nil
}

// RenderYAML is defined on the RenderConfig interface.
func (cfg *WindowsCloudConfig) RenderYAML() ([]byte, error) {
	return cfg.renderWindows()
}

// RenderScript is defined on the RenderConfig interface.
func (cfg *WindowsCloudConfig) RenderScript() (string, error) {
	// NOTE: This shouldn't really be called on windows as it's used only for
	// initialization via ssh or on local providers.
	script, err := cfg.renderWindows()
	if err != nil {
		return "", err
	}

	return string(script), err
}

// getCommandsForAddingPackages is defined on the RenderConfig interface.
func (cfg *WindowsCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	return nil, nil
}

// renderWindows is a helper function which renders the runCmds of the Windows
// CloudConfig to a PowerShell script.
func (cfg *WindowsCloudConfig) renderWindows() ([]byte, error) {
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
func (cfg *WindowsCloudConfig) AddPackageCommands(
	aptProxySettings proxy.Settings,
	aptMirror string,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) {
	return
}

// AddCloudArchiveCloudTools is defined on the AdvancedPackagingConfig
// interface.
func (cfg *WindowsCloudConfig) AddCloudArchiveCloudTools() {
	return
}

// updatePackages is defined on the AdvancedPackagingConfig interface.
func (cfg *WindowsCloudConfig) updatePackages() {
	return
}

// updateProxySettings is defined on the AdvancedPackagingConfig interface.
func (cfg *WindowsCloudConfig) updateProxySettings(proxy.Settings) {
	return
}
