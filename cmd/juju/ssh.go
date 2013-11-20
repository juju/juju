// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"os/exec"
	"time"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

// SSHCommand is responsible for launching a ssh shell on a given unit or machine.
type SSHCommand struct {
	SSHCommon
}

// SSHCommon provides common methods for SSHCommand, SCPCommand and DebugHooksCommand.
type SSHCommon struct {
	cmd.EnvCommandBase
	Target    string
	Args      []string
	apiClient *api.Client
}

const sshDoc = `
Launch an ssh shell on the machine identified by the <target> parameter.
<target> can be either a machine id  as listed by "juju status" in the
"machines" section or a unit name as listed in the "services" section.
Any extra parameters are passsed as extra parameters to the ssh command.

Examples

Connect to machine 0:

    juju ssh 0

Connect to the first mysql unit:

    juju ssh mysql/0
`

func (c *SSHCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ssh",
		Args:    "<target> [<ssh args>...]",
		Purpose: "launch an ssh shell on a given unit or machine",
		Doc:     sshDoc,
	}
}

func (c *SSHCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no target name specified")
	}
	c.Target, c.Args = args[0], args[1:]
	return nil
}

// Run resolves c.Target to a machine, to the address of a i
// machine or unit forks ssh passing any arguments provided.
func (c *SSHCommand) Run(ctx *cmd.Context) error {
	if c.apiClient == nil {
		var err error
		c.apiClient, err = c.initAPIClient()
		if err != nil {
			return err
		}
		defer c.apiClient.Close()
	}
	host, err := c.hostFromTarget(c.Target)
	if err != nil {
		return err
	}
	args := []string{"-l", "ubuntu", "-t", "-o", "StrictHostKeyChecking no", "-o", "PasswordAuthentication no", host}
	args = append(args, c.Args...)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

// initAPIClient initialises the state connection.
// It is the caller's responsibility to close the connection.
func (c *SSHCommon) initAPIClient() (*api.Client, error) {
	var err error
	c.apiClient, err = juju.NewAPIClientFromName(c.EnvName)
	return c.apiClient, err
}

// attemptStarter is an interface corresponding to utils.AttemptStrategy
type attemptStarter interface {
	Start() attempt
}

type attempt interface {
	Next() bool
}

type attemptStrategy utils.AttemptStrategy

func (s attemptStrategy) Start() attempt {
	return utils.AttemptStrategy(s).Start()
}

var sshHostFromTargetAttemptStrategy attemptStarter = attemptStrategy{
	Total: 5 * time.Second,
	Delay: 500 * time.Millisecond,
}

func (c *SSHCommon) hostFromTarget(target string) (string, error) {
	var addr string
	var err error
	// A target may not initially have an address (e.g. the
	// address updater hasn't yet run), so we must do this in
	// a loop.
	for a := sshHostFromTargetAttemptStrategy.Start(); a.Next(); {
		addr, err = c.apiClient.PublicAddress(target)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", err
	}
	logger.Infof("Resolved public address of %q: %q", target, addr)
	return addr, nil
}

// AllowInterspersedFlags for ssh/scp is set to false so that
// flags after the unit name are passed through to ssh, for eg.
// `juju ssh -v service-name/0 uname -a`.
func (c *SSHCommon) AllowInterspersedFlags() bool {
	return false
}
