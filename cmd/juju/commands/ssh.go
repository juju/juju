// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/retry"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	jujussh "github.com/juju/juju/network/ssh"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/statusapi_mock.go github.com/juju/juju/cmd/juju/commands StatusAPI

var usageSSHSummary = `
Initiates an SSH session or executes a command on a Juju machine or container.`[1:]

var usageSSHDetails = `
The ssh target is identified by the <target> argument which is either a 'unit
name' or a 'machine id'. Both can be obtained by examining the output to "juju
status".

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If 'user' is specified then the connection is made to that user
account; otherwise, the default 'ubuntu' account, created by Juju, is used.

The optional command is executed on the remote machine, and any output is sent
back to the user. If no command is specified, then an interactive shell session
will be initiated.

When "juju ssh" is executed without a terminal attached, e.g. when piping the
output of another command into it, then the default behavior is to not allocate
a pseudo-terminal (pty) for the ssh session; otherwise a pty is allocated. This
behavior can be overridden by explicitly specifying the behavior with
"--pty=true" or "--pty=false".

The SSH host keys of the target are verified. The --no-host-key-checks option
can be used to disable these checks. Use of this option is not recommended as
it opens up the possibility of a man-in-the-middle attack.

The default identity known to Juju and used by this command is ~/.ssh/id_rsa

Options can be passed to the local OpenSSH client (ssh) on platforms 
where it is available. This is done by inserting them between the target and 
a possible remote command. Refer to the ssh man page for an explanation 
of those options.

For k8s charms, the --container argument is used to identity a specific
container in the pod. For charms which run the workload in a separate pod
to that of the charm, the default ssh target is the charm operator pod.
The workload pod may be specified using the --remote argument.

Examples:
Connect to machine 0:

    juju ssh 0

Connect to machine 1 and run command 'uname -a':

    juju ssh 1 uname -a

Connect to the leader mysql unit:

    juju ssh mysql/leader

Connect to a specific mysql unit:

    juju ssh mysql/0

Connect to a jenkins unit as user jenkins:

    juju ssh jenkins@jenkins/0

Connect to a mysql unit with an identity not known to juju (ssh option -i):

    juju ssh mysql/0 -i ~/.ssh/my_private_key echo hello

For k8s charms running the workload in a separate pod:
  Connect to a k8s unit targeting the operator pod by default:

	juju ssh mysql/0
	juju ssh mysql/0 bash
	
  Connect to a k8s unit targeting the workload pod by specifying --remote:

	juju ssh --remote mysql/0

For k8s charms using the sidecar pattern:
  Connect to a k8s unit targeting the charm container (the default):

	juju ssh --container charm snappass/0

  Connect to a k8s unit targeting the redis container:

	juju ssh --container redis snappass/0

For k8s controller:
  Connect to the api server pod:

    juju ssh --container api-server 0

  Connect to the mongo db pod:

    juju ssh --container mongodb 0

See also: 
    scp`

const (
	// SSHRetryDelay is the time to wait for an SSH connection to be established
	// to a single endpoint of a target.
	SSHRetryDelay = 500 * time.Millisecond

	// SSHTimeout is the time to wait for before giving up trying to establish
	// an SSH connection to a target, after retrying.
	SSHTimeout = 5 * time.Second
)

func newSSHCommand(
	hostChecker jujussh.ReachableChecker,
	isTerminal func(interface{}) bool,
	retryStrategy retry.CallArgs,
) cmd.Command {
	c := &sshCommand{
		hostChecker:   hostChecker,
		isTerminal:    isTerminal,
		retryStrategy: retryStrategy,
	}
	return modelcmd.Wrap(c)
}

var defaultSSHRetryStrategy = retry.CallArgs{
	Clock:       clock.WallClock,
	MaxDuration: SSHTimeout,
	Delay:       SSHRetryDelay,
}

// sshCommand is responsible for launching a ssh shell on a given unit or machine.
type sshCommand struct {
	modelType model.ModelType
	modelcmd.ModelCommandBase

	sshMachine
	sshContainer

	provider sshProvider

	hostChecker jujussh.ReachableChecker
	isTerminal  func(interface{}) bool
	pty         autoBoolValue

	retryStrategy retry.CallArgs
}

func (c *sshCommand) SetFlags(f *gnuflag.FlagSet) {
	c.sshMachine.SetFlags(f)
	c.sshContainer.SetFlags(f)
	f.Var(&c.pty, "pty", "Enable pseudo-tty allocation")
}

func (c *sshCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "ssh",
		Args:    "<[user@]target> [openssh options] [command]",
		Purpose: usageSSHSummary,
		Doc:     usageSSHDetails,
	})
}

func (c *sshCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("no target name specified")
	}
	if c.modelType, err = c.ModelType(); err != nil {
		return err
	}
	if c.modelType == model.CAAS {
		c.provider = &c.sshContainer
	} else {
		c.provider = &c.sshMachine
	}
	c.provider.setTarget(args[0])
	c.provider.setArgs(args[1:])
	c.provider.setHostChecker(c.hostChecker)
	c.provider.setRetryStrategy(c.retryStrategy)
	return nil
}

// ModelCommand defines methods of the model command.
type ModelCommand interface {
	NewControllerAPIRoot() (api.Connection, error)
	ModelDetails() (string, *jujuclient.ModelDetails, error)
	NewAPIRoot() (api.Connection, error)
	NewAPIClient() (*api.Client, error)
	ModelIdentifier() (string, error)
}

// sshProvider is implemented by either either a CaaS or IaaS model instance.
type sshProvider interface {
	initRun(ModelCommand) error
	cleanupRun()
	setHostChecker(checker jujussh.ReachableChecker)
	resolveTarget(string) (*resolvedTarget, error)
	maybePopulateTargetViaField(*resolvedTarget, func([]string) (*params.FullStatus, error)) error
	ssh(ctx Context, enablePty bool, target *resolvedTarget) error
	copy(Context) error

	getTarget() string
	setTarget(target string)

	getArgs() []string
	setArgs(Args []string)

	setRetryStrategy(retry.CallArgs)
}

// Run resolves c.Target to a machine, to the address of a i
// machine or unit forks ssh passing any arguments provided.
func (c *sshCommand) Run(ctx *cmd.Context) error {
	if err := c.provider.initRun(&c.ModelCommandBase); err != nil {
		return errors.Trace(err)
	}
	defer c.provider.cleanupRun()

	target, err := c.provider.resolveTarget(c.provider.getTarget())
	if err != nil {
		return err
	}

	if c.proxy {
		// If we are trying to connect to a container on a FAN address,
		// we need to route the traffic via the machine that hosts it.
		// This is required as the controller is unable to route fan
		// traffic across subnets.
		if err = c.provider.maybePopulateTargetViaField(target, c.statusClient.Status); err != nil {
			return errors.Trace(err)
		}
	}

	var pty bool
	if c.pty.b != nil {
		pty = *c.pty.b
	} else {
		// Flag was not specified: create a pty
		// on the remote side if this process
		// has a terminal.
		isTerminal := isTerminal
		if c.isTerminal != nil {
			isTerminal = c.isTerminal
		}
		pty = isTerminal(ctx.Stdin)
	}
	return c.provider.ssh(ctx, pty, target)
}

// autoBoolValue is like gnuflag.boolValue, but remembers
// whether or not a value has been set, so its behaviour
// can be determined dynamically, during command execution.
type autoBoolValue struct {
	b *bool
}

func (b *autoBoolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	b.b = &v
	return nil
}

func (b *autoBoolValue) Get() interface{} {
	if b.b != nil {
		return *b.b
	}
	return b.b // nil
}

func (b *autoBoolValue) String() string {
	if b.b != nil {
		return fmt.Sprint(*b.b)
	}
	return "<auto>"
}

func (b *autoBoolValue) IsBoolFlag() bool { return true }

// StatusAPI is implemented by types that can query the full status of a model.
type StatusAPI interface {
	Status([]string) (*params.FullStatus, error)
	Close() error
}

func maybeResolveLeaderUnit(statusAPIGetter func() (StatusAPI, error), target string) (string, error) {
	if !strings.HasSuffix(target, "/leader") {
		return target, nil
	}

	api, err := statusAPIGetter()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() { _ = api.Close() }()

	app := strings.Split(target, "/")[0]
	res, err := api.Status([]string{app})
	if err != nil {
		return "", errors.Annotatef(err, "unable to resolve leader unit for application %q", app)
	}

	for unitName, unitStatus := range res.Applications[app].Units {
		if unitStatus.Leader {
			return unitName, nil
		}
	}

	// Check if we are trying to look up a subordinate leader. Unfortunately,
	// this means we have to iterate each principal unit's subordinate list.
	subordinateAppPrefix := app + "/"
	for _, appInfo := range res.Applications {
		for _, unitStatus := range appInfo.Units {
			for subordinateUnitName, subordinateUnitStatus := range unitStatus.Subordinates {
				if strings.HasPrefix(subordinateUnitName, subordinateAppPrefix) && subordinateUnitStatus.Leader {
					return subordinateUnitName, nil
				}
			}
		}
	}

	return "", errors.Errorf("unable to resolve leader unit for application %q", app)
}
