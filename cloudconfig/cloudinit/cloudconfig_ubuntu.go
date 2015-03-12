// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import "github.com/juju/juju/cloudconfig/cloudinit/packaging"

// UbuntuCloudConfig is the cloudconfig type specific to Ubuntu machines
// It simply contains a cloudConfig with the added package management-related
// methods for the Ubuntu version of cloudinit.
// It satisfies the cloudinit.CloudConfig interface
type UbuntuCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy implements PackageProxyConfig.
func (cfg *UbuntuCloudConfig) SetPackageProxy(url string) {
	cfg.SetAttr("apt_proxy", url)
}

// UnsetPackageProxy implements PackageProxyConfig.
func (cfg *UbuntuCloudConfig) UnsetPackageProxy() {
	cfg.UnsetAttr("apt_proxy")
}

// PackageProxy implements PackageProxyConfig.
func (cfg *UbuntuCloudConfig) PackageProxy() string {
	proxy, _ := cfg.attrs["apt_proxy"].(string)
	return proxy
}

// SetPackageMirror implements PackageMirrorConfig.
func (cfg *UbuntuCloudConfig) SetPackageMirror(url string) {
	cfg.SetAttr("apt_mirror", url)
}

// UnsetPackageMirror implements PackageMirrorConfig.
func (cfg *UbuntuCloudConfig) UnsetPackageMirror() {
	cfg.UnsetAttr("apt_mirror")
}

// PackageMirror implements PackageMirrorConfig.
func (cfg *UbuntuCloudConfig) PackageMirror() string {
	mirror, _ := cfg.attrs["apt_mirror"].(string)
	return mirror
}

// AddPackageSource implements PackageSourcesConfig.
func (cfg *UbuntuCloudConfig) AddPackageSource(src packaging.Source) {
	cfg.attrs["apt_sources"] = append(cfg.PackageSources(), src)
}

// PackageSources implements PackageSourcesConfig.
func (cfg *UbuntuCloudConfig) PackageSources() []packaging.Source {
	srcs, _ := cfg.attrs["apt_sources"].([]packaging.Source)
	return srcs
}

// AddPackagePreferences implements PackageSourcesConfig.
func (cfg *UbuntuCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	cfg.AddBootTextFile(prefs.Path, prefs.FileContents(), 0644)
}
