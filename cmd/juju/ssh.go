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
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

// SSHCommand is responsible for launching a ssh shell on a given unit or machine.
type SSHCommand struct {
	SSHCommon
}

// SSHCommon provides common methods for SSHCommand, SCPCommand and DebugHooksCommand.
type SSHCommon struct {
	envcmd.EnvCommandBase
	proxy     bool
	pty       bool
	Target    string
	Args      []string
	apiClient *api.Client
	apiAddr   string
}

func (c *SSHCommon) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.proxy, "proxy", true, "proxy through the API server")
	f.BoolVar(&c.pty, "pty", true, "enable pseudo-tty allocation")
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
	options.SetProxyCommand(juju, "ssh", "--proxy=false", "--pty=false", apiServerHost, "nc", "-q0", "%h", "%p")
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

// getSSHOptions configures and returns SSH options and proxy settings.
func (c *SSHCommon) getSSHOptions(enablePty bool) (*ssh.Options, error) {
	var options ssh.Options
	if enablePty {
		options.EnablePTY()
	}
	var err error
	if c.proxy, err = c.proxySSH(); err != nil {
		return nil, err
	} else if c.proxy {
		if err := c.setProxyCommand(&options); err != nil {
			return nil, err
		}
	}
	return &options, nil
}

// Run resolves c.Target to a machine, to the address of a i
// machine or unit forks ssh passing any arguments provided.
func (c *SSHCommand) Run(ctx *cmd.Context) error {
	if c.apiClient == nil {
		// If the apClient is not already opened and it is opened
		// by ensureAPIClient, then close it when we're done.
		defer func() {
			if c.apiClient != nil {
				c.apiClient.Close()
				c.apiClient = nil
			}
		}()
	}
	options, err := c.getSSHOptions(c.pty)
	if err != nil {
		return err
	}
	host, err := c.hostFromTarget(c.Target)
	if err != nil {
		return err
	}
	cmd := ssh.Command("ubuntu@"+host, c.Args, options)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

// proxySSH returns true iff both c.proxy and
// the proxy-ssh environment configuration
// are true.
func (c *SSHCommon) proxySSH() (bool, error) {
	if !c.proxy {
		return false, nil
	}
	if _, err := c.ensureAPIClient(); err != nil {
		return false, err
	}
	var cfg *config.Config
	attrs, err := c.apiClient.EnvironmentGet()
	if err == nil {
		cfg, err = config.New(config.NoDefaults, attrs)
	}
	if err != nil {
		return false, err
	}
	logger.Debugf("proxy-ssh is %v", cfg.ProxySSH())
	return cfg.ProxySSH(), nil
}

func (c *SSHCommon) ensureAPIClient() (*api.Client, error) {
	if c.apiClient != nil {
		return c.apiClient, nil
	}
	return c.initAPIClient()
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

func (c *SSHCommon) hostFromTarget(target string) (string, error) {
	// If the target is neither a machine nor a unit,
	// assume it's a hostname and try it directly.
	if !names.IsMachine(target) && !names.IsUnit(target) {
		return target, nil
	}
	// A target may not initially have an address (e.g. the
	// address updater hasn't yet run), so we must do this in
	// a loop.
	if _, err := c.ensureAPIClient(); err != nil {
		return "", err
	}
	var err error
	for a := sshHostFromTargetAttemptStrategy.Start(); a.Next(); {
		var addr string
		if c.proxy {
			addr, err = c.apiClient.PrivateAddress(target)
		} else {
			addr, err = c.apiClient.PublicAddress(target)
		}
		if err == nil {
			return addr, nil
		}
	}
	return "", err
}

// AllowInterspersedFlags for ssh/scp is set to false so that
// flags after the unit name are passed through to ssh, for eg.
// `juju ssh -v service-name/0 uname -a`.
func (c *SSHCommon) AllowInterspersedFlags() bool {
	return false
}
