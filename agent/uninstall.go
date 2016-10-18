// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/juju/names.v2"
)

const (
	// UninstallFile is the name of the file inside the data dir that,
	// if it exists, will cause the machine agent to uninstall itself
	// when it receives the termination signal or ErrTerminateAgent.
	UninstallFile = "uninstall-agent"
)

// WARNING: this is linked with the use of UninstallFile in the
// provider/manual package. Don't change it without extreme care,
// and handling for mismatches with already-deployed agents.
func uninstallFile(a Agent) string {
	return filepath.Join(a.CurrentConfig().DataDir(), UninstallFile)
}

// SetCanUninstall creates the uninstall file in the data dir. It does
// nothing if the supplied agent doesn't have a machine tag.
func SetCanUninstall(a Agent) error {
	tag := a.CurrentConfig().Tag()
	if _, ok := tag.(names.MachineTag); !ok {
		logger.Debugf("cannot uninstall non-machine agent %q", tag)
		return nil
	}
	logger.Infof("marking agent ready for uninstall")
	return ioutil.WriteFile(uninstallFile(a), nil, 0644)
}

// CanUninstall returns true if the uninstall file exists in the agent's
// data dir. If it encounters an error, it fails safe and returns false.
func CanUninstall(a Agent) bool {
	if _, err := os.Stat(uninstallFile(a)); err != nil {
		logger.Debugf("agent not marked ready for uninstall")
		return false
	}
	logger.Infof("agent already marked ready for uninstall")
	return true
}
