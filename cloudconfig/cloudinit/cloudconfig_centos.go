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
	"github.com/juju/utils/proxy"
	"gopkg.in/yaml.v1"
)

// CentOSCloudConfig is the cloudconfig type specific to CentOS machines.
// It simply contains a cloudConfig and adds the package management related
// methods for CentOS, which are mostly modeled as runcmds.
// It implements the cloudinit.Config interface.
type CentOSCloudConfig struct {
	*cloudConfig
	pacman *packaging.PackageManager
	common *unixCloudConfig
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
	cfg.attrs["package_sources"] = append(cfg.PackageSources(), src)
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

// Render implements the Renderer interface.
func (cfg *CentOSCloudConfig) RenderYAML() ([]byte, error) {
	// check for package proxy setting and add commands:
	if proxy := cfg.PackageProxy(); proxy != "" {
		addPackageProxyCmds(cfg, proxy)
		cfg.UnsetPackageProxy()
	}

	// check for package mirror settings and add commands:
	if mirror := cfg.PackageMirror(); mirror != "" {
		addPackageMirrorCmds(cfg, mirror)
		cfg.UnsetPackageMirror()
	}

	// add appropriate commands for package sources configuration:
	for _, src := range cfg.PackageSources() {
		addPackageSourceCmds(cfg, src)
		cfg.UnsetAttr("package_sources")
	}

	data, err := yaml.Marshal(cfg.getAttrs())
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

func (cfg *CentOSCloudConfig) RenderScript() (string, error) {
	//TODO: &cfg?
	return cfg.common.renderScriptCommon(cfg)
}

func (cfg *CentOSCloudConfig) AddPackageCommands(
	aptProxySettings proxy.Settings,
	aptMirror string,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) {
	cfg.common.addPackageCommandsCommon(
		//TODO &cfg?
		cfg,
		aptProxySettings,
		aptMirror,
		addUpdateScripts,
		addUpgradeScripts,
		cfg.series,
	)
}

//TODO: add this to CentOS at the right time
func (cfg *CentOSCloudConfig) MaybeAddCloudArchiveCloudTools() {
}

func (cfg *CentOSCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	var cmds []string

	// If a mirror is specified, rewrite sources.list and rename cached index files.
	//if newMirror, _ := cfg.PackageMirror(); newMirror != "" {
	if newMirror := cfg.PackageMirror(); newMirror != "" {
		cmds = append(cmds, LogProgressCmd("Changing package mirror to "+newMirror))
		// TODO: Change mirror on CentOS?
	}

	// TODO: Do we need this on CentOS?
	//if len(cfg.PackageSources()) > 0 {
	//Ensure add-apt-repository is available.
	//cmds = append(cmds, LogProgressCmd("Installing add-apt-repository"))
	//cmds = append(cmds, pacman.Install("python-software-properties"))
	//}
	for _, src := range cfg.PackageSources() {
		// PPA keys are obtained by add-apt-repository, from launchpad.
		// TODO: Repo keys on CentOS?
		//if !strings.HasPrefix(src.Url, "ppa:") {
		//if src.Key != "" {
		//key := utils.ShQuote(src.Key)
		//cmd := fmt.Sprintf("printf '%%s\\n' %s | apt-key add -", key)
		//cmds = append(cmds, cmd)
		//}
		//}
		cmds = append(cmds, LogProgressCmd("Adding yum repository: %s", src.Url))
		cmds = append(cmds, cfg.pacman.AddRepository(src.Url))
		//TODO: Package prefs on CentOS?
		// if src.Prefs != nil {
		//	path := utils.ShQuote(src.Prefs.Path)
		//	contents := utils.ShQuote(src.Prefs.FileContents())
		//	cmds = append(cmds, "install -D -m 644 /dev/null "+path)
		//	cmds = append(cmds, `printf '%s\n' `+contents+` > `+path)
		//}
	}

	// Define the "apt_get_loop" function, and wrap apt-get with it.
	// TODO: Do we need this on CentOS?
	//cmds = append(cmds, aptgetLoopFunction)
	//aptget = "apt_get_loop " + aptget

	if cfg.SystemUpdate() {
		cmds = append(cmds, LogProgressCmd("Running yum update"))
		cmds = append(cmds, cfg.pacman.Update())
	}
	if cfg.SystemUpgrade() {
		cmds = append(cmds, LogProgressCmd("Running yum upgrade"))
		cmds = append(cmds, cfg.pacman.Upgrade())
	}

	pkgs := cfg.Packages()
	for _, pkg := range pkgs {
		// TODO: Do we need some sort of hacks on CentOS?
		cmds = append(cmds, LogProgressCmd("Installing package: %s", pkg))
		cmds = append(cmds, cfg.pacman.Install(pkg))
	}
	// TODO: wat?
	//if len(cmds) > 0 {
	//setting DEBIAN_FRONTEND=noninteractive prevents debconf
	//from prompting, always taking default values instead.
	//cmds = append([]string{"export DEBIAN_FRONTEND=noninteractive"}, cmds...)
	//}
	return cmds, nil

}

func (cfg *CentOSCloudConfig) updatePackages() {
	list := []string{
		"curl",
		"cpu-checker",
		// TODO(axw) 2014-07-02 #1277359
		// Don't install bridge-utils in cloud-init;
		// leave it to the networker worker.
		"bridge-utils",
		"rsyslog-gnutls",
		"cloud-utils",
		"cloud-image-utils",
	}
	cfg.common.updatePackagesCommon(cfg, list, cfg.series)
}

//TODO: is this the same as doing addPackageProxyCommands?
// either way implement something
// This may replace the SetProxy func. See parent for info
func (cfg *CentOSCloudConfig) updateProxySettings(proxySettings proxy.Settings) {
}
