// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"fmt"

	"github.com/juju/juju/cloudconfig/cloudinit/packaging"
)

// CentOSCloudConfig is the cloudconfig type specific to CentOS machines.
// It simply contains a cloudConfig and adds the package management related
// methods for CentOS, which are mostly modeled as runcmds.
// It implements the cloudinit.Config interface.
type CentOSCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy implements PackageProxyConfig.
func (cfg *CentOSCloudConfig) SetPackageProxy(url string) {
	cfg.SetAttr("package_proxy", url)
}

// setPackageProxy is a helper function which adds the corresponding runcmd
// to apply the package proxy settings on a CentOS machine.
func addPackageProxyCmds(cfg CloudConfig, url string) {
	cfg.AddRunCmd(fmt.Sprintf("/bin/echo 'proxy=%s' >> /etc/yum.conf", url))
}

// UnsetPackageProxy implements PackageProxyConfig.
func (cfg *CentOSCloudConfig) UnsetPackageProxy() {
	cfg.UnsetAttr("package_proxy")
}

// PackageProxy implements PackageProxyConfig.
func (cfg *CentOSCloudConfig) PackageProxy() string {
	proxy, _ := cfg.attrs["package_proxy"].(string)
	return proxy
}

// SetPackageMirror implements PackageMirrorConfig.
func (cfg *CentOSCloudConfig) SetPackageMirror(url string) {
	cfg.SetAttr("package_mirror", url)
}

// setPackageMirror is a helper function that adds the corresponding runcmds
// to apply the package mirror settings on a CentOS machine.
func addPackageMirrorCmds(cfg CloudConfig, url string) {
	cfg.AddRunCmd(fmt.Sprintf(`sed -r -i 's|^mirrorlist|#mirrorlist|g' %s`, packaging.CentOSSourcesFile))
	cfg.AddRunCmd(fmt.Sprintf(`sed -r -i 's|#baseurl=.*|baseurl=%s|g' %s`,
		url, packaging.CentOSSourcesFile))
}

// UnsetPackageMirror implements PackageMirrorConfig.
func (cfg *CentOSCloudConfig) UnsetPackageMirror() {
	cfg.UnsetAttr("package_mirror")
}

// PackageMirror implements PackageMirrorConfig.
func (cfg *CentOSCloudConfig) PackageMirror() string {
	mirror, _ := cfg.attrs["package_mirror"].(string)
	return mirror
}

// AddPackageSource implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) AddPackageSource(src packaging.Source) {
	cfg.attrs["package_source"] = append(cfg.PackageSources(), src)
}

// addPackageSourceCmds is a helper function that adds the corresponding
// runcmds to apply the package source settings on a CentOS machine.
func addPackageSourceCmds(cfg CloudConfig, src packaging.Source) {
	// if keyfile is required, add it first
	if src.Key != "" {
		cfg.AddRunTextFile(src.KeyfilePath(), src.Key, 0644)
	}

	cfg.AddRunTextFile(packaging.CentOSSourcesDir, src.RenderCentOS(), 0644)
}

// PackageSources implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) PackageSources() []packaging.Source {
	sources, _ := cfg.attrs["package_sources"].([]packaging.Source)
	return sources
}

// AddPackagePreferences implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	// TODO (aznashwan): research a way of using yum-priorities in the
	// context of a single package and implement the appropriate runcmds.
}
