// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cloudconfig/cloudinit/packaging"
	"github.com/juju/utils"
)

const (
	// aptSourcesList is the location of the APT sources list
	// configuration file.
	aptSourcesList = "/etc/apt/sources.list"

	// aptListsDirectory is the location of the APT lists directory.
	aptListsDirectory = "/var/lib/apt/lists"

	// extractAptSource is a shell command that will extract the
	// currently configured APT source location. We assume that
	// the first source for "main" in the file is the one that
	// should be replaced throughout the file.
	extractAptSource = `awk "/^deb .* $(lsb_release -sc) .*main.*\$/{print \$2;exit}" ` + aptSourcesList

	// aptSourceListPrefix is a shell program that translates an
	// APT source (piped from stdin) to a file prefix. The algorithm
	// involves stripping up to one trailing slash, stripping the
	// URL scheme prefix, and finally translating slashes to
	// underscores.
	aptSourceListPrefix = `sed 's,.*://,,' | sed 's,/$,,' | tr / _`

	// aptgetLoopFunction is a bash function that executes its arguments
	// in a loop with a delay until either the command either returns
	// with an exit code other than 100.
	aptgetLoopFunction = `
function apt_get_loop {
    local rc=
    while true; do
        if ($*); then
                return 0
        else
                rc=$?
        fi
        if [ $rc -eq 100 ]; then
		sleep 10s
                continue
        fi
        return $rc
    done
}
`
)

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

func getUbuntuCommandsForAddingPackages(cfg CloudConfig, pacman *packaging.PackageManager) ([]string, error) {
	// the basic command for all apt-get calls
	//		--assume-yes to never prompt for confirmation
	//		--force-confold is passed to dpkg to never overwrite config files
	aptget := "apt-get --assume-yes --option Dpkg::Options::=--force-confold "

	// If apt_get_wrapper is specified, then prepend it to aptget.
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
	//if newMirror, _ := cfg.PackageMirror(); newMirror != "" {
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
		cmds = append(cmds, pacman.Install("python-software-properties"))
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
		cmds = append(cmds, pacman.AddRepository(src.Url))
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
