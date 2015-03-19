// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cloudconfig/cloudinit/packaging"
	"github.com/juju/utils"
	"github.com/juju/utils/apt"
	"github.com/juju/utils/proxy"
	"gopkg.in/yaml.v1"
)

// UbuntuCloudConfig is the cloudconfig type specific to Ubuntu machines
// It simply contains a cloudConfig with the added package management-related
// methods for the Ubuntu version of cloudinit.
// It satisfies the cloudinit.CloudConfig interface
type UbuntuCloudConfig struct {
	*cloudConfig
	pacman *packaging.PackageManager
	common *unixCloudConfig
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

func (cfg *UbuntuCloudConfig) RenderYAML() ([]byte, error) {
	data, err := yaml.Marshal(cfg.attrs)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

func (cfg *UbuntuCloudConfig) RenderScript() (string, error) {
	//TODO: &cfg?
	return cfg.common.renderScriptCommon(cfg)
}

func (cfg *UbuntuCloudConfig) AddPackageCommands(
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

// MaybeAddCloudArchiveCloudTools adds the cloud-archive cloud-tools
// pocket to apt sources, if the series requires it.
func (cfg *UbuntuCloudConfig) MaybeAddCloudArchiveCloudTools() {
	if cfg.series != "precise" {
		// Currently only precise; presumably we'll
		// need to add each LTS in here as they're
		// added to the cloud archive.
		return
	}
	const url = "http://ubuntu-cloud.archive.canonical.com/ubuntu"
	name := fmt.Sprintf("deb %s %s-updates/cloud-tools main", url, cfg.series)
	prefs := packaging.PackagePreferences{
		Path:        CloudToolsPrefsPath,
		Explanation: "Pin with lower priority, not to interfere with charms",
		Package:     "*",
		Pin:         fmt.Sprintf("release n=%s-updates/cloud-tools", cfg.series),
		Priority:    400,
	}
	cfg.AddPackagePreferences(prefs)
	source := packaging.Source{
		Url: name,
		Key: CanonicalCloudArchiveSigningKey,
	}
	cfg.AddPackageSource(source)
}

// Used inside unixCloudConfig.RenderScript()
func (cfg *UbuntuCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	if !cfg.SystemUpdate() && len(cfg.PackageSources()) > 0 {
		return nil, fmt.Errorf("update sources were specified, but OS updates have been disabled.")
	}

	// the basic command for all apt-get calls
	//		--assume-yes to never prompt for confirmation
	//		--force-confold is passed to dpkg to never overwrite config files
	aptget := "apt-get --assume-yes --option Dpkg::Options::=--force-confold "

	// If apt_get_wrapper is specified, then prepend it to aptget.
	//TODO: so we're deleting this?
	//aptget := aptget
	//wrapper := cfg.AptGetWrapper()
	//switch wrapper.Enabled {
	//case true:
	//aptget = utils.ShQuote(wrapper.Command) + " " + aptget
	//case "auto":
	//aptget = fmt.Sprintf("$(which %s || true) %s", utils.ShQuote(wrapper.Command), aptget)
	//}

	var cmds []string

	// If a mirror is specified, rewrite sources.list and rename cached index files.
	if newMirror := cfg.PackageMirror(); newMirror != "" {
		cmds = append(cmds, LogProgressCmd("Changing apt mirror to "+newMirror))
		cmds = append(cmds, "old_mirror=$("+extractAptSource+")")
		cmds = append(cmds, "new_mirror="+newMirror)
		cmds = append(cmds, `sed -i s,$old_mirror,$new_mirror, `+aptSourcesList)
		cmds = append(cmds, renameAptListFilesCommands("$new_mirror", "$old_mirror")...)
	}

	if len(cfg.PackageSources()) > 0 {
		// Ensure add-apt-repository is available.
		cmds = append(cmds, LogProgressCmd("Installing add-apt-repository"))
		cmds = append(cmds, cfg.pacman.Install("python-software-properties"))
	}
	for _, src := range cfg.PackageSources() {
		// PPA keys are obtained by add-apt-repository, from launchpad.
		if !strings.HasPrefix(src.Url, "ppa:") {
			if src.Key != "" {
				key := utils.ShQuote(src.Key)
				cmd := fmt.Sprintf("printf '%%s\\n' %s | apt-key add -", key)
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, LogProgressCmd("Adding apt repository: %s", src.Url))
		//cmds = append(cmds, "add-apt-repository -y "+utils.ShQuote(src.Url))
		cmds = append(cmds, cfg.pacman.AddRepository(src.Url))
		//TODO: Do we keep this?
		// if src.Prefs != nil {
		//	path := utils.ShQuote(src.Prefs.Path)
		//	contents := utils.ShQuote(src.Prefs.FileContents())
		//	cmds = append(cmds, "install -D -m 644 /dev/null "+path)
		//	cmds = append(cmds, `printf '%s\n' `+contents+` > `+path)
		//}
	}

	// Define the "apt_get_loop" function, and wrap apt-get with it.
	// TODO: If we do this hack here we can't use the package manager anymore
	// Maybe wrap it inside packageManager?
	cmds = append(cmds, aptgetLoopFunction)
	aptget = "apt_get_loop " + aptget

	if cfg.SystemUpdate() {
		cmds = append(cmds, LogProgressCmd("Running apt-get update"))
		cmds = append(cmds, aptget+"update")
	}
	if cfg.SystemUpgrade() {
		cmds = append(cmds, LogProgressCmd("Running apt-get upgrade"))
		cmds = append(cmds, aptget+"upgrade")
	}

	pkgs := cfg.Packages()
	skipNext := 0
	for i, pkg := range pkgs {
		if skipNext > 0 {
			skipNext--
			continue
		}
		// Make sure the cloud-init 0.6.3 hack (for precise) where
		// --target-release and precise-updates/cloud-tools are
		// specified as separate packages is converted to a single
		// package argument below.
		if pkg == "--target-release" {
			// There has to be at least 2 more items - the target
			// release (e.g. "precise-updates/cloud-tools") and the
			// package name.
			if i+2 >= len(pkgs) {
				remaining := strings.Join(pkgs[:i], " ")
				return nil, errors.Errorf(
					"invalid package %q: expected --target-release <release> <package>",
					remaining,
				)
			}
			pkg = strings.Join(pkgs[i:i+2], " ")
			skipNext = 2
		}
		cmds = append(cmds, LogProgressCmd("Installing package: %s", pkg))
		cmd := fmt.Sprintf(aptget+"install %s", pkg)
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
	oldPrefix := "old_prefix=" + aptListsDirectory + "/$(echo " + oldMirror + " | " + aptSourceListPrefix + ")"
	newPrefix := "new_prefix=" + aptListsDirectory + "/$(echo " + newMirror + " | " + aptSourceListPrefix + ")"
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

func (cfg *UbuntuCloudConfig) updatePackages() {
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

// This may replace the SetProxy func. See parent for info
func (cfg *UbuntuCloudConfig) updateProxySettings(proxySettings proxy.Settings) {
	// Write out the apt proxy settings
	if (proxySettings != proxy.Settings{}) {
		filename := apt.ConfFile
		cfg.AddBootCmd(fmt.Sprintf(
			`printf '%%s\n' %s > %s`,
			shquote(apt.ProxyContent(proxySettings)),
			filename))
	}
}
