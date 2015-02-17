// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import "github.com/juju/juju/cloudinit/packaging"

// UbuntuCloudConfig is the cloudconfig type specific to Ubuntu machines
// It simply contains a cloudConfig with the added package management-related
// methods for the Ubuntu version of cloudinit.
// It satisfies the cloudinit.Config interface
type UbuntuCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy sets the option of using a proxy server for all
// packaging-related operations with apt
func (cfg *UbuntuCloudConfig) SetPackageProxy(url string) {
	cfg.SetAttr("apt_proxy", url)
}

// UnsetPackageProxy unsets the option set by SetPackageProxy
// If it has not been previously set, no error occurs
func (cfg *UbuntuCloudConfig) UnsetPackageProxy() {
	cfg.UnsetAttr("apt_proxy")
}

// PackageProxy returns the value set by SetPackageProxy
// If it has not been previously set, an empty string is returned
func (cfg *UbuntuCloudConfig) PackageProxy() string {
	proxy, _ := cfg.attrs["apt_proxy"].(string)
	return proxy
}

// SetPackageMirror sets the URL to be used as the apt mirror site
// NOTE: if not set, the URL is selected based on cloud metadata in EC2
func (cfg *UbuntuCloudConfig) SetPackageMirror(url string) {
	cfg.SetAttr("apt_mirror", url)
}

// UnsetPackageMirror unsets the value set with SetPackageMirror
// If it has not previously been set, no error is returned
func (cfg *UbuntuCloudConfig) UnsetPackageMirror() {
	cfg.UnsetAttr("apt_mirror")
}

// PackageMirror returns the package mirror url set with SetPackageMirror
// If it has not previously been set, an empty string will be returned
func (cfg *UbuntuCloudConfig) PackageMirror() string {
	mirror, _ := cfg.attrs["apt_mirror"].(string)
	return mirror
}

// AddPackageSource adds a source to have packages pulled from
func (cfg *UbuntuCloudConfig) AddPackageSource(src *packaging.Source) {
	cfg.attrs["apt_sources"] = append(cfg.PackageSources(), src)
}

// PackageSources returns the list of package sources set with AddPackageSource
// If none are set, the returned slice will be empty
func (cfg *UbuntuCloudConfig) PackageSources() []*packaging.Source {
	srcs, _ := cfg.attrs["apt_sources"].([]*packaging.Source)
	return srcs
}

// AddPackagePreferences adds the necessary bootcmds to install the given
// packaging.Preferences early in *every* boot process
func (cfg *UbuntuCloudConfig) AddPackagePreferences(prefs *packaging.PackagePreferences) {
	cfg.AddBootTextFile(prefs.Path, prefs.FileContents(), 0644)
}
