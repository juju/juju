// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/juju/version"
)

var windowsSuffixOrder = []string{
	".ps1",
	".cmd",
	".bat",
	".exe",
}

func lookPath(hook string) (string, error) {
	hookFile, err := exec.LookPath(hook)
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && os.IsNotExist(ee.Err) {
			return "", &missingHookError{hook}
		}
		return "", err
	}
	return hookFile, nil
}

// searchHook will search, in order, hooks suffixed with extensions
// in windowsSuffixOrder. As windows cares about extensions to determine
// how to execute a file, we will allow several suffixes, with powershell
// being default.
func searchHook(charmDir, hook string) (string, error) {
	hookFile := filepath.Join(charmDir, hook)
	if version.Current.OS != version.Windows {
		// we are not running on windows,
		// there is no need to look for suffixed hooks
		return lookPath(hookFile)
	}
	for _, suffix := range windowsSuffixOrder {
		file := fmt.Sprintf("%s%s", hookFile, suffix)
		foundHook, err := lookPath(file)
		if err != nil {
			if IsMissingHookError(err) {
				// look for next suffix
				continue
			}
			return "", err
		}
		return foundHook, nil
	}
	return "", &missingHookError{hook}
}

// hookCommand constructs an appropriate command to be passed to
// exec.Command(). The exec package uses cmd.exe as default on windows.
// cmd.exe does not know how to execute ps1 files by default, and
// powershell needs a few flags to allow execution (-ExecutionPolicy)
// and propagate error levels (-File). .cmd and .bat files can be run
// directly.
func hookCommand(hook string) []string {
	if version.Current.OS != version.Windows {
		// we are not running on windows,
		// just return the hook name
		return []string{hook}
	}
	if strings.HasSuffix(hook, ".ps1") {
		return []string{
			"powershell.exe",
			"-NonInteractive",
			"-ExecutionPolicy",
			"RemoteSigned",
			"-File",
			hook,
		}
	}
	return []string{hook}
}
