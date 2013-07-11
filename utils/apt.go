// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"os/exec"
)

// Some helpful functions for running apt in a sane way

// TODO: When we have a unit-level lock to avoid multiple unit agents running
//       apt concurrently, we should use that same locking here

// osRunCommand calls cmd.Run, this is used as an overloading point so we can
// test what *would* be run without actually executing another program
func osRunCommand(cmd *exec.Cmd) error {
	return cmd.Run()
}

var runCommand = osRunCommand

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
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), aptGetEnvOptions...)
	return runCommand(cmd)
}
