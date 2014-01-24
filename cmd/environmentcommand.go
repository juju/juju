// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/juju/osenv"
)

const CurrentEnvironmentFilename = "current-environment"

// The purpose of EnvCommandBase is to provide a default member and flag
// setting for commands that deal across different environments.
type EnvCommandBase struct {
	CommandBase
	EnvName string
}

func getCurrentEnvironmentFilePath() string {
	return filepath.Join(osenv.JujuHome(), CurrentEnvironmentFilename)
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

// Write the envName to the file $JUJU_HOME/current-environment file.
func WriteCurrentEnvironment(envName string) error {
	path := getCurrentEnvironmentFilePath()
	err := ioutil.WriteFile(path, []byte(envName+"\n"), 0644)
	if err != nil {
		return fmt.Errorf("unable to write to the environment file: %q, %s", path, err)
	}
	return nil
}

// There is simple ordering for the default environment.  Firstly check the
// JUJU_ENV environment variable.  If that is set, it gets used.  If it isn't
// set, look in the $JUJU_HOME/current-environment file.
func getDefaultEnvironment() string {
	defaultEnv := os.Getenv(osenv.JujuEnvEnvKey)
	if defaultEnv != "" {
		return defaultEnv
	}
	return ReadCurrentEnvironment()
}

func (c *EnvCommandBase) SetFlags(f *gnuflag.FlagSet) {
	defaultEnv := getDefaultEnvironment()
	f.StringVar(&c.EnvName, "e", defaultEnv, "juju environment to operate in")
	f.StringVar(&c.EnvName, "environment", defaultEnv, "")
}

// EnvironName returns the name of the environment for this command
func (c *EnvCommandBase) EnvironName() string {
	return c.EnvName
}
