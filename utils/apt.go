// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"os/exec"
	"strings"

	"launchpad.net/loggo"
)

var aptLogger = loggo.GetLogger("juju.utils.apt")

// Some helpful functions for running apt in a sane way

// osRunCommand calls cmd.Run, this is used as an overloading point so we can
// test what *would* be run without actually executing another program
func osRunCommand(cmd *exec.Cmd) error {
	return cmd.Run()
}

var runCommand = osRunCommand

// osCommandOutput calls cmd.Output, this is used as an overloading point so we
// can test what *would* be run without actually executing another program
func osCommandOutput(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

var commandOutput = osCommandOutput

// This is the default apt-get command used in cloud-init, the various settings
// mean that apt won't actually block waiting for a prompt from the user.
var aptGetCommand = []string{
	"apt-get", "--option=Dpkg::Options::=--force-confold",
	"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
}

// aptEnvOptions are options we need to pass to apt-get to not have it prompt
// the user
var aptGetEnvOptions = []string{"DEBIAN_FRONTEND=noninteractive"}

// AptGetInstall runs 'apt-get install packages' for the packages listed here
func AptGetInstall(packages ...string) error {
	cmdArgs := append([]string(nil), aptGetCommand...)
	cmdArgs = append(cmdArgs, "install")
	cmdArgs = append(cmdArgs, packages...)
	aptLogger.Infof("Running: %s", cmdArgs)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), aptGetEnvOptions...)
	return runCommand(cmd)
}

// AptConfigProxy will consult apt-config about the configured proxy
// settings. If there are no proxy settings configured, an empty string is
// returned.
func AptConfigProxy() (string, error) {
	var (
		out []byte
		err error
	)
	cmd := exec.Command(
		"apt-config",
		"dump",
		"Acquire::http::Proxy",
		"Acquire::https::Proxy",
		"Acquire::ftp::Proxy")
	if out, err = commandOutput(cmd); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
