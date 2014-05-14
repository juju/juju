// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/osenv"
)

const CurrentEnvironmentFilename = "current-environment"

// ErrNoEnvironmentSpecified is returned by commands that operate on
// an environment if there is no current environment, no environment
// has been explicitly specified, and there is no default environment.
var ErrNoEnvironmentSpecified = fmt.Errorf("no environment specified")

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
// set, look in the $JUJU_HOME/current-environment file.  If neither are
// available, read environments.yaml and use the default environment therein.
func getDefaultEnvironment() (string, error) {
	if defaultEnv := os.Getenv(osenv.JujuEnvEnvKey); defaultEnv != "" {
		return defaultEnv, nil
	}
	if currentEnv := ReadCurrentEnvironment(); currentEnv != "" {
		return currentEnv, nil
	}
	envs, err := environs.ReadEnvirons("")
	if err != nil {
		return "", err
	}
	if envs.Default == "" {
		return "", ErrNoEnvironmentSpecified
	}
	return envs.Default, nil
}

// EnvironCommand extends cmd.Command with a SetEnvName method.
type EnvironCommand interface {
	cmd.Command

	// SetEnvName is called prior to the wrapped command's Init method
	// with the active environment name. The environment name is guaranteed
	// to be non-empty at entry of Init.
	SetEnvName(envName string)
}

// EnvCommandBase is a convenience type for embedding in commands
// that wish to implement EnvironCommand.
type EnvCommandBase struct {
	cmd.CommandBase
	EnvName string
}

func (c *EnvCommandBase) SetEnvName(envName string) {
	c.EnvName = envName
}

// Wrap wraps the specified EnvironCommand, returning a Command
// that proxies to each of the EnvironCommand methods.
func Wrap(c EnvironCommand) cmd.Command {
	return &environCommandWrapper{EnvironCommand: c}
}

type environCommandWrapper struct {
	EnvironCommand
	envName string
}

func (w *environCommandWrapper) EnvironName() string {
	return w.envName
}

// ensureEnvName ensures that w.envName is non-empty, or sets it to
// the default environment name. If there is no default environment name,
// then ensureEnvName returns ErrNoEnvironmentSpecified.
func (w *environCommandWrapper) ensureEnvName() error {
	if w.envName != "" {
		return nil
	}
	defaultEnv, err := getDefaultEnvironment()
	if err != nil {
		return err
	}
	w.envName = defaultEnv
	return nil
}

func (w *environCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&w.envName, "e", "", "juju environment to operate in")
	f.StringVar(&w.envName, "environment", "", "")
	w.EnvironCommand.SetFlags(f)
}

func (w *environCommandWrapper) Init(args []string) error {
	if err := w.ensureEnvName(); err != nil {
		return err
	}
	w.SetEnvName(w.envName)
	return w.EnvironCommand.Init(args)
}
