// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	// UninstallFile is the name of the file inside the data dir that,
	// if it exists, will cause a machine agent to uninstall itself
	// when it receives the termination signal or ErrTerminateAgent.
	UninstallFile = "uninstall-agent"
)

func uninstallFile(a Agent) string {
	return filepath.Join(a.CurrentConfig().DataDir(), UninstallFile)
}

// SetCanUninstall creates the uninstall file in the agent's data dir.
func SetCanUninstall(a Agent) error {
	logger.Errorf("marking agent ready for uninstall")
	return ioutil.WriteFile(uninstallFile(a), nil, 0644)
}

// CanUninstall returns true if the uninstall file exists in the agent's
// data dir.
func CanUninstall(a Agent) bool {
	if _, err := os.Stat(uninstallFile(a)); err != nil {
		logger.Debugf("agent not marked ready for uninstall")
		return false
	}
	logger.Infof("agent already marked ready for uninstall")
	return true
}
