// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"launchpad.net/gnuflag"

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
	proxy     bool
	Target    string
	Args      []string
	apiClient *api.Client
	apiAddr   string
	// Only used for compatibility with 1.16
	rawConn *juju.Conn
}

func (c *SSHCommon) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.proxy, "proxy", true, "proxy through the API server")
}

// setProxyCommand sets the proxy command option.
func (c *SSHCommon) setProxyCommand(options *ssh.Options) error {
	apiServerHost, _, err := net.SplitHostPort(c.apiAddr)
	if err != nil {
		return fmt.Errorf("failed to get proxy address: %v", err)
	}
	juju, err := getJujuExecutable()
	if err != nil {
		return fmt.Errorf("failed to get juju executable path: %v", err)
	}
	options.SetProxyCommand(juju, "ssh", "--proxy=false", apiServerHost, "-T", "nc -q0 %h %p")
	return nil
}

const sshDoc = `
Launch an ssh shell on the machine identified by the <target> parameter.
<target> can be either a machine id  as listed by "juju status" in the
"machines" section or a unit name as listed in the "services" section.
Any extra parameters are passsed as extra parameters to the ssh command.

Examples:

Connect to machine 0:

    juju ssh 0

Connect to machine 1 and run 'uname -a':

    juju ssh 1 uname -a

Connect to the first mysql unit:

    juju ssh mysql/0

Connect to the first mysql unit and run 'ls -la /var/log/juju':

    juju ssh mysql/0 ls -la /var/log/juju
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

// getJujuExecutable returns the path to the juju
// executable, or an error if it could not be found.
var getJujuExecutable = func() (string, error) {
	return exec.LookPath(os.Args[0])
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
	var options ssh.Options
	options.EnablePTY()
	if c.proxy {
		if err := c.setProxyCommand(&options); err != nil {
			return err
		}
	}
	cmd := ssh.Command("ubuntu@"+host, c.Args, &options)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

// initAPIClient initialises the API connection.
// It is the caller's responsibility to close the connection.
func (c *SSHCommon) initAPIClient() (*api.Client, error) {
	st, err := juju.NewAPIFromName(c.EnvName)
	if err != nil {
		return nil, err
	}
	c.apiClient = st.Client()
	c.apiAddr = st.Addr()
	return c.apiClient, nil
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
		var addr string
		if c.proxy {
			if addr = instance.SelectInternalAddress(machine.Addresses(), false); addr == "" {
				return "", fmt.Errorf("machine %q has no internal address", machine)
			}
		} else {
			if addr = instance.SelectPublicAddress(machine.Addresses()); addr == "" {
				return "", fmt.Errorf("machine %q has no public address", machine)
			}
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
		var addr string
		var ok bool
		if c.proxy {
			if addr, ok = unit.PrivateAddress(); !ok {
				return "", fmt.Errorf("unit %q has no internal address", unit)
			}
		} else {
			if addr, ok = unit.PublicAddress(); !ok {
				return "", fmt.Errorf("unit %q has no public address", unit)
			}
		}
		return addr, nil
	}
	return "", fmt.Errorf("unknown unit or machine %q", target)
}

func (c *SSHCommon) hostFromTarget(target string) (string, error) {
	// If the target is neither a machine nor a unit,
	// assume it's a hostname and try it directly.
	if !names.IsMachine(target) && !names.IsUnit(target) {
		return target, nil
	}
	var addr string
	var err error
	var useStateConn bool
	// A target may not initially have an address (e.g. the
	// address updater hasn't yet run), so we must do this in
	// a loop.
	for a := sshHostFromTargetAttemptStrategy.Start(); a.Next(); {
		if !useStateConn {
			if c.proxy {
				addr, err = c.apiClient.PrivateAddress(target)
			} else {
				addr, err = c.apiClient.PublicAddress(target)
			}
			if params.IsCodeNotImplemented(err) {
				logger.Infof("API server does not support Client.PrivateAddress falling back to 1.16 compatibility mode (direct DB access)")
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
