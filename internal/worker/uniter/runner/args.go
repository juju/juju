// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/worker/common/charmrunner"
)

func lookPath(hook string) (string, error) {
	hookFile, err := exec.LookPath(hook)
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && os.IsNotExist(ee.Err) {
			return "", charmrunner.NewMissingHookError(hook)
		}
		return "", err
	}
	return hookFile, nil
}

// discoverHookScript will return the name of the script to run for a hook.
// We verify an executable file exists with the same name as the hook.
func discoverHookScript(charmDir, hook string) (string, error) {
	hookFile := filepath.Join(charmDir, hook)
	return lookPath(hookFile)
}

func checkCharmExists(charmDir string) error {
	if _, err := os.Stat(path.Join(charmDir, "metadata.yaml")); os.IsNotExist(err) {
		return errors.New("charm missing from disk")
	} else if err != nil {
		return errors.Annotatef(err, "failed to check for metadata.yaml")
	}
	return nil
}
