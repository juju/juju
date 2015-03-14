// Copyright 2014, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import "github.com/juju/juju/cloudconfig/cloudinit/packaging"

func getCentOSCommandsForAddingPackages(cfg CloudConfig, pacman *packaging.PackageManager) ([]string, error) {
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
		cmds = append(cmds, pacman.AddRepository(src.Url))
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
		cmds = append(cmds, pacman.Update())
	}
	if cfg.SystemUpgrade() {
		cmds = append(cmds, LogProgressCmd("Running yum upgrade"))
		cmds = append(cmds, pacman.Upgrade())
	}

	pkgs := cfg.Packages()
	for _, pkg := range pkgs {
		// TODO: Do we need some sort of hacks on CentOS?
		cmds = append(cmds, LogProgressCmd("Installing package: %s", pkg))
		cmds = append(cmds, pacman.Install(pkg))
	}
	// TODO: wat?
	//if len(cmds) > 0 {
	//setting DEBIAN_FRONTEND=noninteractive prevents debconf
	//from prompting, always taking default values instead.
	//cmds = append([]string{"export DEBIAN_FRONTEND=noninteractive"}, cmds...)
	//}
	return cmds, nil
}
