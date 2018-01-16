// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/juju/worker/common/charmrunner"
)

// searchHook will search, in order, hooks suffixed with extensions
// in windowsSuffixOrder. As windows cares about extensions to determine
// how to execute a file, we will allow several suffixes, with powershell
// being default.
func searchHook(charmDir, hook string) (string, error) {
	hookFile, err := exec.LookPath(filepath.Join(charmDir, hook))
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && os.IsNotExist(ee.Err) {
			return "", charmrunner.NewMissingHookError(hook)
		}
		return "", err
	}
	return hookFile, nil
}
