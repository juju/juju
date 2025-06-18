// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"fmt"
	"strings"

	"github.com/juju/packaging/v4"
	"github.com/juju/packaging/v4/config"
	"gopkg.in/yaml.v2"

	corenetwork "github.com/juju/juju/core/network"
	jujupackaging "github.com/juju/juju/packaging"
)

// PackageHelper is the interface for configuring specific parameter of the package manager
type packageHelper interface {
	// addPackageProxyCmd is a helper method which returns the corresponding runcmd
	// to apply the package proxy settings.
	addPackageProxyCmd(url string) string

	//Returns the required packages, depending on the OS
	getRequiredPackages() []string
}

// Implementation of PackageHelper for CentOS
type centOSHelper struct {
}

// Returns the list of required packages in CentOS
func (helper centOSHelper) getRequiredPackages() []string {
	return []string{
		"curl",
		"nmap-ncat",
		"tmux",
		"tar",
	}
}

// addPackageProxyCmd is a helper method which returns the corresponding runcmd
// to apply the package proxy settings for CentOS
func (helper centOSHelper) addPackageProxyCmd(url string) string {
	return fmt.Sprintf("/bin/echo 'proxy=%s' >> /etc/yum.conf", url)
}

// centOSCloudConfig is the cloudconfig type specific to CentOS machines.
// It simply contains a cloudConfig and adds the package management related
// methods for CentOS, which are mostly modeled as runcmds.
// It implements the cloudinit.Config interface.
type centOSCloudConfig struct {
	*cloudConfig
	helper packageHelper
}

// SetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *centOSCloudConfig) SetPackageProxy(url string) {
	cfg.SetAttr("package_proxy", url)
}

// UnsetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *centOSCloudConfig) UnsetPackageProxy() {
	cfg.UnsetAttr("package_proxy")
}

// PackageProxy is defined on the PackageProxyConfig interface.
func (cfg *centOSCloudConfig) PackageProxy() string {
	proxy, _ := cfg.attrs["package_proxy"].(string)
	return proxy
}

// SetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *centOSCloudConfig) SetPackageMirror(url string) {
	cfg.SetAttr("package_mirror", url)
}

// addPackageMirrorCmd is a helper function that returns the corresponding runcmds
// to apply the package mirror settings on a CentOS machine.
func addPackageMirrorCmd(cfg CloudConfig, url string) string {
	return fmt.Sprintf(config.ReplaceCentOSMirror, url)
}

// UnsetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *centOSCloudConfig) UnsetPackageMirror() {
	cfg.UnsetAttr("package_mirror")
}

// PackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *centOSCloudConfig) PackageMirror() string {
	mirror, _ := cfg.attrs["package_mirror"].(string)
	return mirror
}

// AddPackageSource is defined on the PackageSourcesConfig interface.
func (cfg *centOSCloudConfig) AddPackageSource(src packaging.PackageSource) {
	cfg.attrs["package_sources"] = append(cfg.PackageSources(), src)
}

// PackageSources is defined on the PackageSourcesConfig interface.
func (cfg *centOSCloudConfig) PackageSources() []packaging.PackageSource {
	sources, _ := cfg.attrs["package_sources"].([]packaging.PackageSource)
	return sources
}

// AddPackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *centOSCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	// TODO (aznashwan): research a way of using yum-priorities in the
	// context of a single package and implement the appropriate runcmds.
}

// PackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *centOSCloudConfig) PackagePreferences() []packaging.PackagePreferences {
	// TODO (aznashwan): add this when priorities in yum make sense.
	return []packaging.PackagePreferences{}
}

// Render is defined on the the Renderer interface.
func (cfg *centOSCloudConfig) RenderYAML() ([]byte, error) {
	// Save the fields that we will modify
	var oldruncmds []string
	oldruncmds = copyStringSlice(cfg.RunCmds())

	// check for package proxy setting and add commands:
	var proxy string
	if proxy = cfg.PackageProxy(); proxy != "" {
		cfg.AddRunCmd(cfg.helper.addPackageProxyCmd(proxy))
		cfg.UnsetPackageProxy()
	}

	// check for package mirror settings and add commands:
	var mirror string
	if mirror = cfg.PackageMirror(); mirror != "" {
		cfg.AddRunCmd(addPackageMirrorCmd(cfg, mirror))
		cfg.UnsetPackageMirror()
	}

	// add appropriate commands for package sources configuration:
	srcs := cfg.PackageSources()
	for _, src := range srcs {
		cfg.AddScripts(addPackageSourceCmds(cfg, src)...)
	}
	cfg.UnsetAttr("package_sources")

	data, err := yaml.Marshal(cfg.attrs)
	if err != nil {
		return nil, err
	}

	// Restore the modified fields
	cfg.SetPackageProxy(proxy)
	cfg.SetPackageMirror(mirror)
	cfg.SetAttr("package_sources", srcs)
	if oldruncmds != nil {
		cfg.SetAttr("runcmd", oldruncmds)
	} else {
		cfg.UnsetAttr("runcmd")
	}

	return append([]byte("#cloud-config\n"), data...), nil
}

func (cfg *centOSCloudConfig) RenderScript() (string, error) {
	return renderScriptCommon(cfg)
}

func (cfg *centOSCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	var cmds []string

	if newMirror := cfg.PackageMirror(); newMirror != "" {
		cmds = append(cmds, LogProgressCmd("Changing package mirror does not yet work on CentOS"))
		// TODO(bogdanteleaga, aznashwan): This should work after a further PR
		// where we add more mirrror options values to environs.Config
		cmds = append(cmds, addPackageMirrorCmd(cfg, newMirror))
	}

	pkgCmder := cfg.paccmder[jujupackaging.YumPackageManager]
	for _, src := range cfg.PackageSources() {
		// TODO(bogdanteleaga. aznashwan): Keys are usually offered by repositories, and you need to
		// accept them. Check how this can be done non interactively.
		cmds = append(cmds, LogProgressCmd("Adding yum repository: %s", src.URL))
		cmds = append(cmds, pkgCmder.AddRepositoryCmd(src.URL))
	}

	// TODO(bogdanteleaga. aznashwan): Research what else needs to be done here

	// Define the "package_get_loop" function
	cmds = append(cmds, config.PackageManagerLoopFunction)

	if cfg.SystemUpdate() {
		cmds = append(cmds, LogProgressCmd("Running yum update"))
		cmds = append(cmds, "package_manager_loop "+pkgCmder.UpdateCmd())
	}
	if cfg.SystemUpgrade() {
		cmds = append(cmds, LogProgressCmd("Running yum upgrade"))
		cmds = append(cmds, "package_manager_loop "+pkgCmder.UpgradeCmd())
	}

	pkgs := cfg.Packages()
	if len(pkgs) > 0 {
		cmds = append([]string{LogProgressCmd("Installing %s", strings.Join(pkgs, ", "))}, cmds...)
	}
	for _, pkg := range pkgs {
		cmds = append(cmds, "package_manager_loop "+pkgCmder.InstallCmd(pkg))
	}
	return cmds, nil
}

// AddPackageCommands is defined on the AdvancedPackagingConfig interface.
func (cfg *centOSCloudConfig) AddPackageCommands(
	proxyCfg PackageManagerProxyConfig,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) error {
	return addPackageCommandsCommon(
		cfg,
		proxyCfg,
		addUpdateScripts,
		addUpgradeScripts,
	)
}

// addRequiredPackages is defined on the AdvancedPackagingConfig interface.
func (cfg *centOSCloudConfig) addRequiredPackages() {
	packages := cfg.helper.getRequiredPackages()
	for _, pack := range packages {
		cfg.AddPackage(pack)
	}
}

// TODO(bogdanteleaga, aznashwan): On ubuntu when we render the conf as yaml we
// have apt_proxy and when we render it as bash we use the equivalent of this.
// However on centOS even when rendering the YAML we use a helper function
// addPackageProxyCmds. Research if calling the same is fine.
func (cfg *centOSCloudConfig) updateProxySettings(PackageManagerProxyConfig) error {
	return nil
}

// AddNetworkConfig is defined on the NetworkingConfig interface.
// TODO(wpk) This has to be implemented for CentOS on VSphere to work properly!
func (cfg *centOSCloudConfig) AddNetworkConfig(interfaces corenetwork.InterfaceInfos) error {
	return nil
}
