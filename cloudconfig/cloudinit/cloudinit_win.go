// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"github.com/juju/juju/cloudconfig/cloudinit/packaging"
	"github.com/juju/utils/proxy"
)

// WindowsCloudConfig is the cloudconfig type specific to Ubuntu machines
// It simply contains a cloudConfig with the added package management-related
// methods for the Ubuntu version of cloudinit.
// It satisfies the cloudinit.CloudConfig interface
type WindowsCloudConfig struct {
	*cloudConfig
}

// RenderYAML implements RenderConfig
func (cfg *WindowsCloudConfig) RenderYAML() ([]byte, error) {
	return cfg.renderWindows()
}

// RenderScript implements RenderConfig
// This shouldn't really be called on windows as it's used only for initialization via ssh or on local providers
func (cfg *WindowsCloudConfig) RenderScript() (string, error) {
	script, err := cfg.renderWindows()
	if err != nil {
		return "", err
	}
	//TODO: good enough?
	return string(script), err
}

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

// SetPackageProxy implements PackageProxyConfig.
func (cfg *WindowsCloudConfig) SetPackageProxy(url string) {
	return
}

// UnsetPackageProxy implements PackageProxyConfig.
func (cfg *WindowsCloudConfig) UnsetPackageProxy() {
	return
}

// PackageProxy implements PackageProxyConfig.
func (cfg *WindowsCloudConfig) PackageProxy() string {
	return ""
}

// SetPackageMirror implements PackageMirrorConfig.
func (cfg *WindowsCloudConfig) SetPackageMirror(url string) {
	return
}

// UnsetPackageMirror implements PackageMirrorConfig.
func (cfg *WindowsCloudConfig) UnsetPackageMirror() {
	return
}

// PackageMirror implements PackageMirrorConfig.
func (cfg *WindowsCloudConfig) PackageMirror() string {
	return ""
}

// AddPackageSource implements PackageSourcesConfig.
func (cfg *WindowsCloudConfig) AddPackageSource(src packaging.Source) {
	return
}

// PackageSources implements PackageSourcesConfig.
func (cfg *WindowsCloudConfig) PackageSources() []packaging.Source {
	return nil
}

// AddPackagePreferences implements PackageSourcesConfig.
func (cfg *WindowsCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	return
}

func (cfg *WindowsCloudConfig) AddPackageCommands(
	aptProxySettings proxy.Settings,
	aptMirror string,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) {
	return
}

func (cfg *WindowsCloudConfig) MaybeAddCloudArchiveCloudTools() {
	return
}

func (cfg *WindowsCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	return nil, nil
}

func (cfg *WindowsCloudConfig) updatePackages() {
	return
}

func (cfg *WindowsCloudConfig) updateProxySettings(proxy.Settings) {
	return
}
