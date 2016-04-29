// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/ssh"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/sshclient"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageSSHSummary = `
Initiates an SSH session or executes a command on a Juju machine.`[1:]

var usageSSHDetails = `
The machine is identified by the <target> argument which is either a 'unit
name' or a 'machine id'. Both are obtained in the output to `[1:] + "`juju status`" + `.
If 'user' is specified then the connection is made to that user account;
otherwise, the default 'ubuntu' account, created by Juju, is used.
The optional command is executed on the remote machine. Any output is sent
back to the user. Screen-based programs require the default of '--pty=true'.

Examples:
Connect to machine 0:

    juju ssh 0

Connect to machine 1 and run command 'uname -a':

    juju ssh 1 uname -a

Connect to a mysql unit:

    juju ssh mysql/0

Connect to a jenkins unit as user jenkins:

    juju ssh jenkins@jenkins/0

See also: 
    scp`

func newSSHCommand() cmd.Command {
	return modelcmd.Wrap(&sshCommand{})
}

// sshCommand is responsible for launching a ssh shell on a given unit or machine.
type sshCommand struct {
	SSHCommon
}

// SSHCommon provides common methods for sshCommand, SCPCommand and DebugHooksCommand.
type SSHCommon struct {
	modelcmd.ModelCommandBase
	proxy     bool
	pty       bool
	Target    string
	Args      []string
	apiClient sshAPIClient
	apiAddr   string
}

func (c *SSHCommon) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.proxy, "proxy", false, "Proxy through the API server")
	f.BoolVar(&c.pty, "pty", true, "Enable pseudo-tty allocation")
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
	options.SetProxyCommand(juju, "ssh", "--proxy=false", "--pty=false", apiServerHost, "nc", "%h", "%p")
	return nil
}

func (c *sshCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ssh",
		Args:    "<[user@]target> [command]",
		Purpose: usageSSHSummary,
		Doc:     usageSSHDetails,
	}
}

func (c *sshCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no target name specified")
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

	// TODO(waigani) do not save fingerprint only until this bug is addressed:
	// lp:892552. Also see lp:1334481.
	options.SetKnownHostsFile("/dev/null")
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
func (c *sshCommand) Run(ctx *cmd.Context) error {
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

	user, host, err := c.userHostFromTarget(c.Target)
	if err != nil {
		return err
	}
	cmd := ssh.Command(user+"@"+host, c.Args, options)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

// proxySSH returns false if both c.proxy and
// the proxy-ssh environment configuration
// are false -- otherwise it returns true.
func (c *SSHCommon) proxySSH() (bool, error) {
	if _, err := c.ensureAPIClient(); err != nil {
		return false, errors.Trace(err)
	}
	proxy, err := c.apiClient.Proxy()
	if err != nil {
		return false, errors.Trace(err)
	}
	logger.Debugf("proxy-ssh is %v", proxy)
	return proxy || c.proxy, nil
}

func (c *SSHCommon) ensureAPIClient() (sshAPIClient, error) {
	if c.apiClient != nil {
		return c.apiClient, nil
	}
	return c.initAPIClient()
}

// initAPIClient initialises the API connection.
// It is the caller's responsibility to close the connection.
func (c *SSHCommon) initAPIClient() (sshAPIClient, error) {
	st, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	c.apiClient = sshclient.NewFacade(st)
	c.apiAddr = st.Addr()
	return c.apiClient, nil
}

type sshAPIClient interface {
	PublicAddress(target string) (string, error)
	PrivateAddress(target string) (string, error)
	Proxy() (bool, error)
	Close() error
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

func (c *SSHCommon) userHostFromTarget(target string) (user, host string, err error) {
	if i := strings.IndexRune(target, '@'); i != -1 {
		user = target[:i]
		target = target[i+1:]
	} else {
		user = "ubuntu"
	}

	// If the target is neither a machine nor a unit,
	// assume it's a hostname and try it directly.
	if !names.IsValidMachine(target) && !names.IsValidUnit(target) {
		return user, target, nil
	}

	// A target may not initially have an address (e.g. the
	// address updater hasn't yet run), so we must do this in
	// a loop.
	if _, err := c.ensureAPIClient(); err != nil {
		return "", "", err
	}
	for a := sshHostFromTargetAttemptStrategy.Start(); a.Next(); {
		var addr string
		if c.proxy {
			addr, err = c.apiClient.PrivateAddress(target)
		} else {
			addr, err = c.apiClient.PublicAddress(target)
		}
		if err == nil {
			return user, addr, nil
		}
	}
	return "", "", err
}

// AllowInterspersedFlags for ssh/scp is set to false so that
// flags after the unit name are passed through to ssh, for eg.
// `juju ssh -v service-name/0 uname -a`.
func (c *SSHCommon) AllowInterspersedFlags() bool {
	return false
}
