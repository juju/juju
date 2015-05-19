// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/juju/osenv"
)

const (
	CurrentEnvironmentFilename = "current-environment"
	CurrentSystemFilename      = "current-system"
)

func getCurrentEnvironmentFilePath() string {
	return filepath.Join(osenv.JujuHome(), CurrentEnvironmentFilename)
}

func getCurrentSystemFilePath() string {
	return filepath.Join(osenv.JujuHome(), CurrentSystemFilename)
}

// Read the file $JUJU_HOME/current-environment and return the value stored
// there.  If the file doesn't exist, or there is a problem reading the file,
// an empty string is returned.
func ReadCurrentEnvironment() string {
	current, err := ioutil.ReadFile(getCurrentEnvironmentFilePath())
	// The file not being there, or not readable isn't really an error for us
	// here.  We treat it as "can't tell, so you get the default".
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(current))
}

// Read the file $JUJU_HOME/current-system and return the value stored
// there.  If the file doesn't exist, or there is a problem reading the file,
// an empty string is returned.
//
// probably want to add error returns...
func ReadCurrentSystem() string {
	current, err := ioutil.ReadFile(getCurrentSystemFilePath())
	// The file not being there, or not readable isn't really an error for us
	// here.  We treat it as "can't tell, so you get the default".
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(current))
}

// Write the envName to the file $JUJU_HOME/current-environment file.
func WriteCurrentEnvironment(envName string) error {
	path := getCurrentEnvironmentFilePath()
	err := ioutil.WriteFile(path, []byte(envName+"\n"), 0644)
	if err != nil {
		return errors.Errorf("unable to write to the environment file: %q, %s", path, err)
	}
	// If there is a current system file, remove it.
	if err := os.Remove(getCurrentSystemFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Write the systemName to the file $JUJU_HOME/current-system file.
func WriteCurrentSystem(systemName string) error {
	path := getCurrentSystemFilePath()
	err := ioutil.WriteFile(path, []byte(systemName+"\n"), 0644)
	if err != nil {
		return errors.Errorf("unable to write to the environment file: %q, %s", path, err)
	}
	// If there is a current environment file, remove it.
	if err := os.Remove(getCurrentEnvironmentFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
