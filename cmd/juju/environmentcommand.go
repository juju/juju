package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
)

const CurrentEnvironmentFile = "current-environment"

// The purpose of EnvCommandBase is to provide a default member and flag
// setting for commands that deal across different environments.
type EnvCommandBase struct {
	cmd.CommandBase
	EnvName string
}

// Read the file $JUJU_HOME/current-environment and return the value stored
// there.  If the file doesn't exist, or there is a problem reading the file,
// an empty string is returned.
func readCurrentEnvironment() string {
	currentEnvironment := filepath.Join(config.JujuHome(), CurrentEnvironmentFile)
	current, err := ioutil.ReadFile(currentEnvironment)
	// The file not being there, or not readable isn't really an error for us
	// here.  We treat it as "can't tell, so you get the default".
	if err != nil {
		return ""
	}
	return string(current)
}

// There is simple ordering for the default environment.  Firstly check the
// JUJU_ENV environment variable.  If that is set, it gets used.  If it isn't
// set, look in the $JUJU_HOME/current-environment file.
func getDefaultEnvironment() string {
	defaultEnv := os.Getenv("JUJU_ENV")
	if defaultEnv != "" {
		return defaultEnv
	}
	return readCurrentEnvironment()
}

func (c *EnvCommandBase) SetFlags(f *gnuflag.FlagSet) {
	defaultEnv := getDefaultEnvironment()
	f.StringVar(&c.EnvName, "e", defaultEnv, "juju environment to operate in")
	f.StringVar(&c.EnvName, "environment", defaultEnv, "")
}
