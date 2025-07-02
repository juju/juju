// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/snap"
	"github.com/juju/juju/internal/packaging/config"
	"github.com/juju/juju/internal/packaging/source"
)

// ubuntuCloudConfig is the cloudconfig type specific to Ubuntu machines
// It simply contains a cloudConfig with the added package management-related
// methods for the Ubuntu version of cloudinit.
// It satisfies the cloudinit.CloudConfig interface
type ubuntuCloudConfig struct {
	*cloudConfig
}

// SetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *ubuntuCloudConfig) SetPackageProxy(url string) {
	cfg.SetAttr("apt_proxy", url)
}

// UnsetPackageProxy is defined on the PackageProxyConfig interface.
func (cfg *ubuntuCloudConfig) UnsetPackageProxy() {
	cfg.UnsetAttr("apt_proxy")
}

// PackageProxy is defined on the PackageProxyConfig interface.
func (cfg *ubuntuCloudConfig) PackageProxy() string {
	p, _ := cfg.attrs["apt_proxy"].(string)
	return p
}

// SetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *ubuntuCloudConfig) SetPackageMirror(url string) {
	cfg.SetAttr("apt_mirror", url)
}

// UnsetPackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *ubuntuCloudConfig) UnsetPackageMirror() {
	cfg.UnsetAttr("apt_mirror")
}

// PackageMirror is defined on the PackageMirrorConfig interface.
func (cfg *ubuntuCloudConfig) PackageMirror() string {
	mirror, _ := cfg.attrs["apt_mirror"].(string)
	return mirror
}

// AddPackageSource is defined on the PackageSourcesConfig interface.
func (cfg *ubuntuCloudConfig) AddPackageSource(src source.PackageSource) {
	cfg.attrs["apt_sources"] = append(cfg.PackageSources(), src)
}

// PackageSources is defined on the PackageSourcesConfig interface.
func (cfg *ubuntuCloudConfig) PackageSources() []source.PackageSource {
	srcs, _ := cfg.attrs["apt_sources"].([]source.PackageSource)
	return srcs
}

// AddPackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *ubuntuCloudConfig) AddPackagePreferences(prefs source.PackagePreferences) {
	cfg.attrs["apt_preferences"] = append(cfg.PackagePreferences(), prefs)
}

// PackagePreferences is defined on the PackageSourcesConfig interface.
func (cfg *ubuntuCloudConfig) PackagePreferences() []source.PackagePreferences {
	prefs, _ := cfg.attrs["apt_preferences"].([]source.PackagePreferences)
	return prefs
}

func (cfg *ubuntuCloudConfig) RenderYAML() ([]byte, error) {
	// Save the fields that we will modify
	oldbootcmds := copyStringSlice(cfg.BootCmds())

	// apt_preferences is not a valid field so we use a fake field in attrs
	// and then render it differently
	prefs := cfg.PackagePreferences()
	for _, pref := range prefs {
		prefFile, err := cfg.aptConfigurer.RenderPreferences(pref)
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

	// Restore the modified fields
	cfg.SetAttr("apt_preferences", prefs)
	if oldbootcmds != nil {
		cfg.SetAttr("bootcmd", oldbootcmds)
	} else {
		cfg.UnsetAttr("bootcmd")
	}

	return append([]byte("#cloud-config\n"), data...), nil
}

func (cfg *ubuntuCloudConfig) RenderScript() (string, error) {
	return renderScriptCommon(cfg)
}

// AddPackageCommands is defined on the AdvancedPackagingConfig interface.
func (cfg *ubuntuCloudConfig) AddPackageCommands(
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

// getCommandsForAddingPackages is a helper function for generating a script
// for adding all packages configured in this CloudConfig.
func (cfg *ubuntuCloudConfig) getCommandsForAddingPackages() ([]string, error) {
	if !cfg.SystemUpdate() && len(cfg.PackageSources()) > 0 {
		return nil, errors.New("update sources were specified, but OS updates have been disabled.")
	}

	var cmds []string

	// If a mirror is specified, rewrite sources.list and rename cached index files.
	if newMirror := cfg.PackageMirror(); newMirror != "" {
		cmds = append(cmds, LogProgressCmd("Changing apt mirror to %q", newMirror))
		cmds = append(cmds, cfg.aptCommander.SetMirrorCommands(newMirror, newMirror)...)
	}

	if len(cfg.PackageSources()) > 0 {
		// Ensure add-apt-repository is available.
		cmds = append(cmds, LogProgressCmd("Installing add-apt-repository"))
		cmds = append(cmds, cfg.aptCommander.InstallCmd("software-properties-common"))
	}
	for _, src := range cfg.PackageSources() {
		// PPA keys are obtained by add-apt-repository, from launchpad.
		if !strings.HasPrefix(src.URL, "ppa:") {
			if src.Key != "" {
				key := utils.ShQuote(src.Key)
				cmd := fmt.Sprintf("echo %s | apt-key add -", key)
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, LogProgressCmd("Adding apt repository: %s", src.URL))
		cmds = append(cmds, cfg.aptCommander.AddRepositoryCmd(src.URL))
	}

	for _, prefs := range cfg.PackagePreferences() {
		prefFile, err := cfg.aptConfigurer.RenderPreferences(prefs)
		if err != nil {
			return nil, err
		}
		cfg.AddRunTextFile(prefs.Path, prefFile, 0644)
	}

	cmds = append(cmds, config.PackageManagerLoopFunction)
	looper := "package_manager_loop "

	if cfg.SystemUpdate() {
		cmds = append(cmds, LogProgressCmd("Running apt-get update"))
		cmds = append(cmds, looper+cfg.aptCommander.UpdateCmd())
	}
	if cfg.SystemUpgrade() {
		cmds = append(cmds, LogProgressCmd("Running apt-get upgrade"))
		cmds = append(cmds, looper+"apt-mark hold cloud-init")
		cmds = append(cmds, looper+cfg.aptCommander.UpgradeCmd())
		cmds = append(cmds, looper+"apt-mark unhold cloud-init")
	}

	var pkgCmds []string
	var pkgNames []string
	var pkgsWithTargetRelease []string
	pkgs := cfg.Packages()
	for i := range pkgs {
		pack := pkgs[i]
		if pack == "--target-release" || len(pkgsWithTargetRelease) > 0 {
			// We have --target-release foo/bar package. Accumulate
			// the args until we've reached the package, before
			// passing the 3 element slice to InstallCmd below.
			pkgsWithTargetRelease = append(pkgsWithTargetRelease, pack)
			if len(pkgsWithTargetRelease) < 3 {
				// We expect exactly 3 elements, the last one being
				// the package.
				continue
			}
		}
		pkgNames = append(pkgNames, pack)
		installArgs := []string{pack}

		if len(pkgsWithTargetRelease) == 3 {
			// If we have a --target-release package, build the
			// install command args from the accumulated
			// pkgsWithTargetRelease slice and reset it.
			installArgs = append([]string{}, pkgsWithTargetRelease...)
			pkgsWithTargetRelease = []string{}
		}

		cmd := looper + cfg.aptCommander.InstallCmd(installArgs...)
		pkgCmds = append(pkgCmds, cmd)
	}

	if len(pkgCmds) > 0 {
		pkgCmds = append([]string{LogProgressCmd("Installing %s", strings.Join(pkgNames, ", "))}, pkgCmds...)
		cmds = append(cmds, pkgCmds...)
		// setting DEBIAN_FRONTEND=noninteractive prevents debconf
		// from prompting, always taking default values instead.
		cmds = append([]string{"export DEBIAN_FRONTEND=noninteractive"}, cmds...)
	}

	return cmds, nil

}

// addRequiredPackages is defined on the AdvancedPackagingConfig interface.
func (cfg *ubuntuCloudConfig) addRequiredPackages() {
	packages := []string{
		"curl",
		"tmux",
	}
	for _, pack := range packages {
		cfg.AddPackage(pack)
	}
}

var waitSnapSeeded = `
n=1
while true; do

echo "Attempt $n to wait for snapd to be seeded...\n"
snap wait core seed.loaded && break
if [ $n -eq 5 ]; then
  echo "snapd not initialised"
  break
fi

echo "Wait for snapd failed, retrying in 5s"
sleep 5
n=$((n+1))
done
`[1:]

// Updates proxy settings used when rendering the conf as a script
func (cfg *ubuntuCloudConfig) updateProxySettings(proxyCfg PackageManagerProxyConfig) error {
	// Write out the apt proxy settings
	if aptProxy := proxyCfg.AptProxy(); (aptProxy != proxy.Settings{}) {
		filename := config.AptProxyConfigFile
		cfg.AddBootCmd(fmt.Sprintf(
			`echo %s > %s`,
			utils.ShQuote(cfg.aptCommander.ProxyConfigContents(aptProxy)),
			filename))
	}

	once := false
	addWaitSnapSeeded := func() {
		if once {
			return
		}
		cfg.AddRunCmd(waitSnapSeeded)
		once = true
	}
	// Write out the snap http/https proxy settings
	if snapProxy := proxyCfg.SnapProxy(); (snapProxy != proxy.Settings{}) {
		addWaitSnapSeeded()
		proxyCommands, err := cfg.snapCommander.SetProxyCmds(snapProxy)
		if err != nil {
			return err
		}
		for _, cmd := range proxyCommands {
			cfg.AddRunCmd(cmd)
		}
	}

	// Configure snap store proxy
	if proxyURL := proxyCfg.SnapStoreProxyURL(); proxyURL != "" {
		assertions, storeID, err := snap.LookupAssertions(proxyURL)
		if err != nil {
			return err
		}
		logger.Infof(context.TODO(), "auto-detected snap store assertions from proxy")
		logger.Infof(context.TODO(), "auto-detected snap store ID as %q", storeID)
		addWaitSnapSeeded()
		cfg.genSnapStoreProxyCmds(assertions, storeID)
	} else if proxyCfg.SnapStoreAssertions() != "" && proxyCfg.SnapStoreProxyID() != "" {
		addWaitSnapSeeded()
		cfg.genSnapStoreProxyCmds(proxyCfg.SnapStoreAssertions(), proxyCfg.SnapStoreProxyID())
	}

	return nil
}

func (cfg *ubuntuCloudConfig) genSnapStoreProxyCmds(assertions, storeID string) {
	cfg.AddRunTextFile("/etc/snap.assertions", assertions, 0600)
	cfg.AddRunCmd("snap ack /etc/snap.assertions")
	cfg.AddRunCmd("rm /etc/snap.assertions")
	cfg.AddRunCmd("snap set core proxy.store=" + storeID)
}
