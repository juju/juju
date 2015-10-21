package main

import (
	"os"
	"os/exec"

	"github.com/juju/cmd"
)

func (c *restoreCommand) runNewRestore(ctx *cmd.Context) error {
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
