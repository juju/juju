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
	cfg.AddRunCmd(fmt.Sprintf("/bin/echo 'proxy=%s' >> /etc/yum.conf", url))
}

// UnsetPackageProxy implements PackageProxyConfig.
func (cfg *CentOSCloudConfig) UnsetPackageProxy() {
	cfg.attrs["runcmds"] = removeRegexpFromSlice(cfg.RunCmds(), `/bin/echo 'proxy=.*' >> /etc/yum\.conf`)
}

// PackageProxy implements PackageProxyConfig.
func (cfg *CentOSCloudConfig) PackageProxy() string {
	found := extractRegexpsFromSlice(cfg.RunCmds(), `/bin/echo \'proxy=(.*)\' >> /etc/yum\.conf`)

	if len(found) == 0 {
		return ""
	} else {
		return found[0]
	}
}

// SetPackageMirror implements PackageMirrorConfig.
func (cfg *CentOSCloudConfig) SetPackageMirror(url string) {
	cfg.AddRunCmd(fmt.Sprintf(`sed -r -i 's|^mirrorlist|#mirrorlist|g' %s`, packaging.CentOSSourcesFile))
	cfg.AddRunCmd(fmt.Sprintf(`sed -r -i 's|#baseurl=.*|baseurl=%s|g' %s`,
		url, packaging.CentOSSourcesFile))
}

// UnsetPackageMirror implements PackageMirrorConfig.
func (cfg *CentOSCloudConfig) UnsetPackageMirror() {
	cfg.attrs["runcmds"] = removeRegexpFromSlice(cfg.RunCmds(), fmt.Sprintf(".*(%s)$", packaging.CentOSSourcesFile))
}

// PackageMirror implements PackageMirrorConfig.
func (cfg *CentOSCloudConfig) PackageMirror() string {
	found := extractRegexpsFromSlice(cfg.RunCmds(), ".*\\|baseurl=(.*)/..releasever.*")

	if len(found) == 0 {
		return ""
	} else {
		return found[0]
	}
}

// AddPackageSource implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) AddPackageSource(src packaging.Source) {
	// if keyfile is required, add it first
	if src.Key != "" {
		cfg.AddRunTextFile(src.KeyfilePath(), src.Key, 0644)
	}

	cfg.AddRunTextFile(packaging.CentOSSourcesDir, src.RenderCentOS(), 0644)
}

// PackageSources implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) PackageSources() []packaging.Source {
	sources := []packaging.Source{}

	for _, source := range extractRegexpsFromSlice(cfg.RunCmds(), "install -D -m 644 /dev/null (.*)") {
		sources = append(sources, packaging.Source{Name: source})
	}

	return sources
}

// AddPackagePreferences implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	// TODO (aznashwan): research a way of using yum-priorities in
	// the context of a single package.
}
