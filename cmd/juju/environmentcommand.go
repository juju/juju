// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
)

const CurrentEnvironmentFilename = "current-environment"

// The purpose of EnvCommandBase is to provide a default member and flag
// setting for commands that deal across different environments.
type EnvCommandBase struct {
	cmd.CommandBase
	EnvName string
}

func getCurrentEnvironmentFilePath() string {
	return filepath.Join(config.JujuHome(), CurrentEnvironmentFilename)
}

// Read the file $JUJU_HOME/current-environment and return the value stored
// there.  If the file doesn't exist, or there is a problem reading the file,
// an empty string is returned.
func readCurrentEnvironment() string {
	current, err := ioutil.ReadFile(getCurrentEnvironmentFilePath())
	// The file not being there, or not readable isn't really an error for us
	// here.  We treat it as "can't tell, so you get the default".
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(current))
}

// Write the envName to the file $JUJU_HOME/current-environment file.
func writeCurrentEnvironment(envName string) error {
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

// envOpenFailure checks to see if the given error is a NoEnvError, and if it is, and
// the caller was using the
func (c *EnvCommandBase) envOpenFailure(err error, w io.Writer) error {
	if errors.IsNoEnv(err) {
		if c.EnvName == "" {
			fmt.Fprintln(w, "No juju environment configuration file exists.")
			fmt.Fprintln(w, err.Error())
			fmt.Fprintln(w, "Please create a configuration by running:")
			fmt.Fprintln(w, "    juju init -w")
			fmt.Fprintln(w, "then edit the file to configure your juju environment.")
			fmt.Fprintln(w, "You can then re-run the command.")
		} else {
			fmt.Fprintln(w, "Juju environment configuration file does not exist.")
			fmt.Fprintln(w, err.Error())
		}
		return cmd.ErrSilent
	}
	return err
}
