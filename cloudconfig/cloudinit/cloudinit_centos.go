// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"fmt"

	"github.com/juju/utils/packaging"
	"github.com/juju/utils/packaging/configurer"
	"github.com/juju/utils/proxy"
	"gopkg.in/yaml.v1"
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

// addPackageProxyCmd is a helper function which returns the corresponding runcmd
// to apply the package proxy settings on a CentOS machine.
func addPackageProxyCmd(cfg CloudConfig, url string) string {
	return fmt.Sprintf("/bin/echo 'proxy=%s' >> /etc/yum.conf", url)
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

// addPackageMirrorCmd is a helper function that returns the corresponding runcmds
// to apply the package mirror settings on a CentOS machine.
func addPackageMirrorCmd(cfg CloudConfig, url string) string {
	return fmt.Sprintf(configurer.ReplaceCentOSMirror, url)
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
func (cfg *CentOSCloudConfig) AddPackageSource(src packaging.PackageSource) {
	cfg.attrs["package_sources"] = append(cfg.PackageSources(), src)
}

// PackageSources implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) PackageSources() []packaging.PackageSource {
	sources, _ := cfg.attrs["package_sources"].([]packaging.PackageSource)
	return sources
}

// AddPackagePreferences implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	// TODO (aznashwan): research a way of using yum-priorities in the
	// context of a single package and implement the appropriate runcmds.
}

// PackagePreferences implements PackageSourcesConfig.
func (cfg *CentOSCloudConfig) PackagePreferences() []packaging.PackagePreferences {
	// TODO (aznashwan): add this when priorities in yum make sense.
	return []packaging.PackagePreferences{}
}

// Render implements the Renderer interface.
func (cfg *CentOSCloudConfig) RenderYAML() ([]byte, error) {
	// check for package proxy setting and add commands:
	var proxy string
	if proxy = cfg.PackageProxy(); proxy != "" {
		cfg.AddRunCmd(addPackageProxyCmd(cfg, proxy))
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

	//restore
	//TODO(centos): check that this actually works
	// We have the same thing in ubuntu as well
	cfg.SetPackageProxy(proxy)
	cfg.SetPackageMirror(mirror)
	cfg.SetAttr("package_sources", srcs)

	return append([]byte("#cloud-config\n"), data...), nil
}

func (cfg *CentOSCloudConfig) RenderScript() (string, error) {
	//TODO: &cfg?
	return renderScriptCommon(cfg)
}

// AddCloudArchiveCloudTools implements AdvancedPackagingConfig.
func (cfg *CentOSCloudConfig) AddCloudArchiveCloudTools() {
	src, pref := configurer.GetCloudArchiveSource(cfg.series)
	cfg.AddPackageSource(src)
	cfg.AddPackagePreferences(pref)
}

func (cfg *CentOSCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	var cmds []string

	if newMirror := cfg.PackageMirror(); newMirror != "" {
		cmds = append(cmds, LogProgressCmd("Changing package mirror does not yet work on CentOS"))
		// TODO(centos): This should be done in a further PR once we add more mirror
		// options values to environs.Config
	}

	for _, src := range cfg.PackageSources() {
		// TODO(centos): Keys are usually offered by repositories, and you need to
		// accept them. Check how this can be done non interactively.
		//if !strings.HasPrefix(src.Url, "ppa:") {
		//if src.Key != "" {
		//key := utils.ShQuote(src.Key)
		//cmd := fmt.Sprintf("printf '%%s\\n' %s | apt-key add -", key)
		//cmds = append(cmds, cmd)
		//}
		//}
		cmds = append(cmds, LogProgressCmd("Adding yum repository: %s", src.Url))
		cmds = append(cmds, cfg.paccmder.AddRepositoryCmd(src.Url))
		//TODO: Package prefs on CentOS?
		// if src.Prefs != nil {
		//	path := utils.ShQuote(src.Prefs.Path)
		//	contents := utils.ShQuote(src.Prefs.FileContents())
		//	cmds = append(cmds, "install -D -m 644 /dev/null "+path)
		//	cmds = append(cmds, `printf '%s\n' `+contents+` > `+path)
		//}
	}

	//TODO(centos): Don't forget about PackagePreferences on CentOS
	//for _, pref := range cfg.PackagePreferences() {
	//}

	// Define the "package_get_loop" function
	cmds = append(cmds, configurer.PackageManagerLoopFunction)

	if cfg.SystemUpdate() {
		cmds = append(cmds, LogProgressCmd("Running yum update"))
		cmds = append(cmds, "package_manager_loop "+cfg.paccmder.UpdateCmd())
	}
	if cfg.SystemUpgrade() {
		cmds = append(cmds, LogProgressCmd("Running yum upgrade"))
		cmds = append(cmds, "package_manager_loop "+cfg.paccmder.UpgradeCmd())
	}

	pkgs := cfg.Packages()
	for _, pkg := range pkgs {
		cmds = append(cmds, LogProgressCmd("Installing package: %s", pkg))
		cmds = append(cmds, "package_manager_loop "+cfg.paccmder.InstallCmd(pkg))
	}
	return cmds, nil
}

// AddPackageCommands implements AdvancedPackagingConfig.
func (cfg *CentOSCloudConfig) AddPackageCommands(
	packageProxySettings proxy.Settings,
	packageMirror string,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) {
	addPackageCommandsCommon(
		cfg,
		packageProxySettings,
		packageMirror,
		addUpdateScripts,
		addUpgradeScripts,
		cfg.series,
	)
}

// updatePackages implements AdvancedPackagingConfig.
func (cfg *CentOSCloudConfig) updatePackages() {
	packages := []string{
		"curl",
		"bridge-utils",
		"rsyslog-gnutls",
		"cloud-utils",
	}

	// The required packages need to come from the correct repo.
	// For precise, that might require an explicit repo targeting.
	// We cannot just pass packages below, because
	// this will generate install commands which older
	// versions of cloud-init (e.g. 0.6.3 in precise) will
	// interpret incorrectly (see bug http://pad.lv/1424777).
	for _, pack := range packages {
		if cfg.pacconfer.IsCloudArchivePackage(pack) {
			// On precise, we need to pass a --target-release entry in
			// pieces for it to work:
			for _, p := range cfg.pacconfer.ApplyCloudArchiveTarget(pack) {
				cfg.AddPackage(p)
			}
		} else {
			cfg.AddPackage(pack)
		}
	}
}

//TODO(centos): is this the same as doing addPackageProxyCommands?
// either way implement something
// Ubuntu uses the equivalent for this when rendering cloudInit as a script
// In CentOS we use it in both YAML and bash rendering. We could use the same
// thing for both I guess
func (cfg *CentOSCloudConfig) updateProxySettings(proxySettings proxy.Settings) {
}
