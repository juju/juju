// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file which is Ubuntu compatible.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"fmt"
	"strings"

	"github.com/juju/utils"
	"github.com/juju/utils/packaging"
	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/proxy"
	"gopkg.in/yaml.v1"
)

// UbuntuCloudConfig is the cloudconfig type specific to Ubuntu machines
// It simply contains a cloudConfig with the added package management-related
// methods for the Ubuntu version of cloudinit.
// It satisfies the cloudinit.CloudConfig interface
type UbuntuCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *UbuntuCloudConfig) SetPackageProxy(url string) {
	cfg.SetAttr("apt_proxy", url)
}

// UnsetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *UbuntuCloudConfig) UnsetPackageProxy() {
	cfg.UnsetAttr("apt_proxy")
}

// PackageProxy is defined on the PackageProxyConfig interface.
func (cfg *UbuntuCloudConfig) PackageProxy() string {
	proxy, _ := cfg.attrs["apt_proxy"].(string)
	return proxy
}

// SetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *UbuntuCloudConfig) SetPackageMirror(url string) {
	cfg.SetAttr("apt_mirror", url)
}

// UnsetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *UbuntuCloudConfig) UnsetPackageMirror() {
	cfg.UnsetAttr("apt_mirror")
}

// PackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *UbuntuCloudConfig) PackageMirror() string {
	mirror, _ := cfg.attrs["apt_mirror"].(string)
	return mirror
}

// AddPackageSource is defined on the PackageSourcesConfig interface.
func (cfg *UbuntuCloudConfig) AddPackageSource(src packaging.PackageSource) {
	cfg.attrs["apt_sources"] = append(cfg.PackageSources(), src)
}

// PackageSources is defined on the PackageSourcesConfig interface.
func (cfg *UbuntuCloudConfig) PackageSources() []packaging.PackageSource {
	srcs, _ := cfg.attrs["apt_sources"].([]packaging.PackageSource)
	return srcs
}

// AddPackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *UbuntuCloudConfig) AddPackagePreferences(prefs packaging.PackagePreferences) {
	cfg.attrs["apt_preferences"] = append(cfg.PackagePreferences(), prefs)
}

// PackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *UbuntuCloudConfig) PackagePreferences() []packaging.PackagePreferences {
	prefs, _ := cfg.attrs["apt_preferences"].([]packaging.PackagePreferences)
	return prefs
}

func (cfg *UbuntuCloudConfig) RenderYAML() ([]byte, error) {
	// add the preferences first:
	prefs := cfg.PackagePreferences()
	for _, pref := range prefs {
		prefFile, err := cfg.pacconfer.RenderPreferences(pref)
		if err != nil {
			return nil, err
		}
		cfg.AddBootTextFile(pref.Path, prefFile, 0644)
	}
	cfg.UnsetAttr("apt_preferences")

	data, err := yaml.Marshal(cfg.attrs)
	if err != nil {
		return nil, err
	}

	//restore
	cfg.SetAttr("apt_preferences", prefs)

	return append([]byte("#cloud-config\n"), data...), nil
}

func (cfg *UbuntuCloudConfig) RenderScript() (string, error) {
	return renderScriptCommon(cfg)
}

// AddPackageCommands is defined on the AdvancedPackagingConfig interface.
func (cfg *UbuntuCloudConfig) AddPackageCommands(
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

// AddCloudArchiveCloudTools is defined on the AdvancedPackagingConfig
// interface.
func (cfg *UbuntuCloudConfig) AddCloudArchiveCloudTools() {
	src, pref := config.GetCloudArchiveSource(cfg.series)
	cfg.AddPackageSource(src)
	cfg.AddPackagePreferences(pref)
}

// getCommandsForAddingPackages is a helper function for generating a script
// for adding all packages configured in this CloudConfig.
func (cfg *UbuntuCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	if !cfg.SystemUpdate() && len(cfg.PackageSources()) > 0 {
		return nil, fmt.Errorf("update sources were specified, but OS updates have been disabled.")
	}

	var cmds []string

	// If a mirror is specified, rewrite sources.list and rename cached index files.
	if newMirror := cfg.PackageMirror(); newMirror != "" {
		cmds = append(cmds, LogProgressCmd("Changing apt mirror to "+newMirror))
		cmds = append(cmds, "old_mirror=$("+config.ExtractAptSource+")")
		cmds = append(cmds, "new_mirror="+newMirror)
		cmds = append(cmds, `sed -i s,$old_mirror,$new_mirror, `+config.AptSourcesFile)
		cmds = append(cmds, renameAptListFilesCommands("$new_mirror", "$old_mirror")...)
	}

	if len(cfg.PackageSources()) > 0 {
		// Ensure add-apt-repository is available.
		cmds = append(cmds, LogProgressCmd("Installing add-apt-repository"))
		cmds = append(cmds, cfg.paccmder.InstallCmd("python-software-properties"))
	}
	for _, src := range cfg.PackageSources() {
		// PPA keys are obtained by add-apt-repository, from launchpad.
		if !strings.HasPrefix(src.URL, "ppa:") {
			if src.Key != "" {
				key := utils.ShQuote(src.Key)
				cmd := fmt.Sprintf("printf '%%s\\n' %s | apt-key add -", key)
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, LogProgressCmd("Adding apt repository: %s", src.URL))
		cmds = append(cmds, cfg.paccmder.AddRepositoryCmd(src.URL))
	}

	for _, prefs := range cfg.PackagePreferences() {
		prefFile, err := cfg.pacconfer.RenderPreferences(prefs)
		if err != nil {
			return nil, err
		}
		cfg.AddRunTextFile(prefs.Path, prefFile, 0644)
	}

	cmds = append(cmds, config.PackageManagerLoopFunction)

	looper := "package_manager_loop "

	if cfg.SystemUpdate() {
		cmds = append(cmds, LogProgressCmd("Running apt-get update"))
		cmds = append(cmds, looper+cfg.paccmder.UpdateCmd())
	}
	if cfg.SystemUpgrade() {
		cmds = append(cmds, LogProgressCmd("Running apt-get upgrade"))
		cmds = append(cmds, looper+cfg.paccmder.UpgradeCmd())
	}

	pkgs := cfg.Packages()
	for i, _ := range pkgs {
		pack := pkgs[i]
		// apply --target-release, if required.
		if config.SeriesRequiresCloudArchiveTools(cfg.series) && cfg.pacconfer.IsCloudArchivePackage(pack) {
			pack = strings.Join(cfg.pacconfer.ApplyCloudArchiveTarget(pack), " ")
		}

		cmds = append(cmds, LogProgressCmd("Installing package: %s", pkgs[i]))
		cmd := looper + cfg.paccmder.InstallCmd(pack)
		cmds = append(cmds, cmd)
	}

	if len(cmds) > 0 {
		// setting DEBIAN_FRONTEND=noninteractive prevents debconf
		// from prompting, always taking default values instead.
		cmds = append([]string{"export DEBIAN_FRONTEND=noninteractive"}, cmds...)
	}

	return cmds, nil

}

// renameAptListFilesCommands takes a new and old mirror string,
// and returns a sequence of commands that will rename the files
// in aptListsDirectory.
func renameAptListFilesCommands(newMirror, oldMirror string) []string {
	oldPrefix := "old_prefix=" + config.AptListsDirectory + "/$(echo " + oldMirror + " | " + config.AptSourceListPrefix + ")"
	newPrefix := "new_prefix=" + config.AptListsDirectory + "/$(echo " + newMirror + " | " + config.AptSourceListPrefix + ")"
	renameFiles := `
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    mv $old $new
done`

	return []string{
		oldPrefix,
		newPrefix,
		// Don't do anything unless the mirror/source has changed.
		`[ "$old_prefix" != "$new_prefix" ] &&` + renameFiles,
	}
}

// updatePackages is defined on the AdvancedPackagingConfig interface.
func (cfg *UbuntuCloudConfig) updatePackages() {
	packages := []string{
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

	// The required packages need to come from the correct repo.
	// For precise, that might require an explicit --target-release parameter.
	// We cannot just pass packages below, because
	// this will generate install commands which older
	// versions of cloud-init (e.g. 0.6.3 in precise) will
	// interpret incorrectly (see bug http://pad.lv/1424777).
	for _, pack := range packages {
		if config.SeriesRequiresCloudArchiveTools(cfg.series) && cfg.pacconfer.IsCloudArchivePackage(pack) {
			// On precise, we need to pass a --target-release entry in
			// pieces for it to work:
			// TODO (aznashwan): figure out what the hell precise wants.
			cfg.AddPackage(strings.Join(cfg.pacconfer.ApplyCloudArchiveTarget(pack), " "))
		} else {
			cfg.AddPackage(pack)
		}
	}
}

// Updates proxy settings used when rendering the conf as a script
func (cfg *UbuntuCloudConfig) updateProxySettings(proxySettings proxy.Settings) {
	// Write out the apt proxy settings
	if (proxySettings != proxy.Settings{}) {
		filename := config.AptProxyConfigFile
		cfg.AddBootCmd(fmt.Sprintf(
			`printf '%%s\n' %s > %s`,
			shquote(cfg.paccmder.ProxyConfigContents(proxySettings)),
			filename))
	}
}
