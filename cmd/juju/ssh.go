// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"time"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
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
	// Only used for compatibility with 1.16
	rawConn *juju.Conn
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
	args := c.Args
	if len(args) > 0 && args[0] == "--" {
		// utils/ssh adds "--"; we will continue to accept
		// it from the CLI for backwards compatibility.
		args = args[1:]
	}
	var options ssh.Options
	options.EnablePTY()
	cmd := ssh.Command("ubuntu@"+host, args, &options)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

// initAPIClient initialises the API connection.
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

// ensureRawConn ensures that c.rawConn is valid (or returns an error)
// This is only for compatibility with a 1.16 API server (that doesn't have
// some of the API added more recently.) It can be removed once we no longer
// need compatibility with direct access to the state database
func (c *SSHCommon) ensureRawConn() error {
	if c.rawConn != nil {
		return nil
	}
	var err error
	c.rawConn, err = juju.NewConnFromName(c.EnvName)
	return err
}

func (c *SSHCommon) hostFromTarget1dot16(target string) (string, error) {
	err := c.ensureRawConn()
	if err != nil {
		return "", err
	}
	// is the target the id of a machine ?
	if names.IsMachine(target) {
		logger.Infof("looking up address for machine %s...", target)
		// This is not the exact code from the 1.16 client
		// (machinePublicAddress), however it is the code used in the
		// apiserver behind the PublicAddress call. (1.16 didn't know
		// about SelectPublicAddress)
		// The old code watched for changes on the Machine until it had
		// an InstanceId and then would return the instance.WaitDNS()
		machine, err := c.rawConn.State.Machine(target)
		if err != nil {
			return "", err
		}
		addr := instance.SelectPublicAddress(machine.Addresses())
		if addr == "" {
			return "", fmt.Errorf("machine %q has no public address", machine)
		}
		return addr, nil
	}
	// maybe the target is a unit ?
	if names.IsUnit(target) {
		logger.Infof("looking up address for unit %q...", c.Target)
		unit, err := c.rawConn.State.Unit(target)
		if err != nil {
			return "", err
		}
		addr, ok := unit.PublicAddress()
		if !ok {
			return "", fmt.Errorf("unit %q has no public address", unit)
		}
		return addr, nil
	}
	return "", fmt.Errorf("unknown unit or machine %q", target)
}

func (c *SSHCommon) hostFromTarget(target string) (string, error) {
	var addr string
	var err error
	var useStateConn bool
	// A target may not initially have an address (e.g. the
	// address updater hasn't yet run), so we must do this in
	// a loop.
	for a := sshHostFromTargetAttemptStrategy.Start(); a.Next(); {
		if !useStateConn {
			addr, err = c.apiClient.PublicAddress(target)
			if params.IsCodeNotImplemented(err) {
				logger.Infof("API server does not support Client.PublicAddress falling back to 1.16 compatibility mode (direct DB access)")
				useStateConn = true
			}
		}
		if useStateConn {
			addr, err = c.hostFromTarget1dot16(target)
		}
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
