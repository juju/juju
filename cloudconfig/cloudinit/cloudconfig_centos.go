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

// CentOSCloudConfig is the cloudconfig type specific to CentOS machines
// It simply contains a cloudConfig and adds the package management related
// methods for CentOS, which are mostly modeled as runcmds
// It satisfies the cloudinit.Config interface
type CentOSCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy satisfies the cloudinit.PackageProxyConfig interface
func (cfg *CentOSCloudConfig) SetPackageProxy(url string) {
	cfg.AddRunCmd(fmt.Sprintf("/bin/echo 'proxy=%s' >> /etc/yum.conf", url))
}

// UnsetPackageProxy satisfies the cloudinit.PackageProxyConfig interface
func (cfg *CentOSCloudConfig) UnsetPackageProxy() {
	cfg.attrs["runcmds"] = removeRegexpFromSlice(cfg.RunCmds(), `/bin/echo 'proxy=.*' >> /etc/yum\.conf`)
}

// PackageProxy satisfies the cloudinit.PackageProxyConfig interface
func (cfg *CentOSCloudConfig) PackageProxy() string {
	found := extractRegexpsFromSlice(cfg.RunCmds(), `/bin/echo \'proxy=(.*)\' >> /etc/yum\.conf`)

	if len(found) == 0 {
		return ""
	} else {
		return found[0]
	}
}

// SetPackageMirror satisfies the cloudinit.PackageMirrorConfig interface
func (cfg *CentOSCloudConfig) SetPackageMirror(url string) {
	cfg.AddRunCmd(fmt.Sprintf(`sed -r -i 's|^mirrorlist|#mirrorlist|g'`, packaging.CentOSSourcesFile))
	cfg.AddRunCmd(fmt.Sprintf(`sed -r -i 's|#baseurl=http://mirror.centos.org/(.*)/\$releasever/(.*)/\$basearch/|baseurl=%s/\$releasever/\2/\$basearch/|g' %s`,
		url, packaging.CentOSSourcesFile))
}

// UnsetPackageMirror satisfies the cloudinit.PackageMirrorConfig interface
func (cfg *cloudConfig) UnsetPackageMirror() {
	cfg.attrs["runcmds"] = removeRegexpFromSlice(cfg.RunCmds(), fmt.Sprintf(".*(%s)$", packaging.CentOSSourcesFile))
}

// PackageMirror satisfies the cloudinit.PackageMirrorConfig interface
func (cfg *cloudConfig) PackageMirror() string {
	found := extractRegexpsFromSlice(cfg.RunCmds(), ".*\\|baseurl=(.*)/..releasever.*")

	if len(found) == 0 {
		return ""
	} else {
		return found[0]
	}
}

// AddPackageSource satisfies the cloudinit.PackageSourcesConfig
func (cfg *cloudConfig) AddPackageSource(src packaging.Source) {
	pm := packaging.CentOSPackageManager()
	cfg.AddRunCmd(pm.AddRepository(src.Url))
}

// PackageSources satisfies the cloudinit.PackageSourcesConfig interface
func (cfg *cloudConfig) PackageSources() []packaging.Source {
	sources := []packaging.Source{}
	pm := packaging.CentOSPackageManager()

	for _, source := range extractRegexpsFromSlice(cfg.RunCmds(), pm.AddRepository("(.*)")) {
		sources = append(sources, packaging.Source{Url: source})
	}

	return sources
}
