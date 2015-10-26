// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"

	"github.com/juju/juju/version"

	"github.com/juju/cmd"
)

func (c *restoreCommand) runRestore(ctx *cmd.Context) error {
	cmdArgs := []string{"backups"}
	if c.Log.Path != "" {
		cmdArgs = append(cmdArgs, "--log-file", c.Log.Path)
	}
	if c.Log.Verbose {
		cmdArgs = append(cmdArgs, "--verbose")
	}
	if c.Log.Quiet {
		cmdArgs = append(cmdArgs, "--quiet")
	}
	if c.Log.Debug {
		cmdArgs = append(cmdArgs, "--debug")
	}
	if c.Log.Config != c.Log.DefaultConfig {
		cmdArgs = append(cmdArgs, "--logging-config", c.Log.Config)
	}
	if c.Log.ShowLog {
		cmdArgs = append(cmdArgs, "--show-log")
	}

	cmdArgs = append(cmdArgs, "restore", "-b", "--file", c.backupFile)
	cmd := exec.Command("juju", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *restoreCommand) supportsNewRestore(ctx *cmd.Context) bool {
	cmd := exec.Command("juju", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Errorf("cannot run juju version: %vr", err)
		return false
	}
	output := out.String()
	output = strings.TrimSpace(output)
	ver, err := version.ParseBinary(output)
	if err != nil {
		logger.Errorf("cannot parse juju version: %v", err)
		// if we cant parse the version number the version might
		// as well not be compatible.
		return false
	}
	// 1.25.0 is the minor version that will work certainly with
	// the new restore.
	restoreAvailableVersion := version.Number{
		Major: 1,
		Minor: 25,
		Patch: 0,
	}
	logger.Infof("current juju version is %q", output)
	return ver.Number.Compare(restoreAvailableVersion) >= 0
}
