// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/retry"
	"github.com/mattn/go-isatty"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/jujuclient"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	jujussh "github.com/juju/juju/internal/network/ssh"
)

var usageSSHSummary = `
Initiates an SSH session or executes a command on a Juju machine or container.`[1:]

var usageSSHDetails = `
By default, this command connects through the controller's SSH jump server.
Choose a target from ` + "`juju status`" + `: a top-level machine number (e.g. ` + "`0`" + `)
or a unit name (e.g. ` + "`mysql/0`" + ` or ` + "`mysql/leader`" + `). In a Kubernetes model,
use a unit name, not a machine number.

For a machine or machine-backed unit, a command runs as ` + "`ubuntu`" + `; omit the
command to open a shell. A ` + "`user@`" + ` prefix does not change this backend login.
For a Kubernetes unit, the command runs in the ` + "`charm`" + ` container by default.
Use ` + "`--container`" + ` to choose another container in the same pod. With no
command the client requests a Bash login shell. If Bash is not installed,
specify a shell the image provides, for example ` + "`juju ssh mysql/0 sh`" + `.

When ` + "`juju ssh`" + ` is executed without a terminal attached, e.g., when piping the
output of another command into it, then the default behavior is to not allocate
a pseudo-terminal (pty) for the ssh session; otherwise a pty is allocated. This
behavior can be overridden by explicitly specifying the behavior with
` + "`--pty=true`" + ` or ` + "`--pty=false`" + `.

Select the private key for the jump server with ` + "`--ssh-key`" + `. To pass options
to the local OpenSSH client, put them between the target and the remote
command. An ` + "`-i`" + ` option there does not select the jump identity.

For an older controller that does not support the jump route, specify
` + "`--direct`" + `; the client does not switch automatically. On machine models this
uses ordinary SSH, where ` + "`user@target`" + ` selects a remote account. On Kubernetes
it uses the Kubernetes API rather than the controller's SSH server.

On machine models, use ` + "`--direct --proxy`" + ` to route the connection through
the controller machine's OpenSSH server (port 22 by default). In direct mode,
if ` + "`--proxy`" + ` is omitted, Juju uses the model's ` + "`proxy-ssh`" + ` setting.
Use ` + "`--direct --proxy=false`" + ` to connect without this proxy.

On the jump route, the client checks both server host keys against keys
obtained from the Juju API by default. Disabling checks with ` + "`--no-host-key-checks`" + `
risks connecting to an impostor. Use ` + "`--show-command`" + ` to print an OpenSSH
invocation rather than run the jump session. When you run it, OpenSSH uses its own host-key
configuration, not the client's temporary known-hosts file.

Use ` + "`--print-keys`" + ` to display both expected SHA256 fingerprints from the
authenticated Juju API without opening a target session. Compare each
OpenSSH trust prompt with the fingerprint for that hop.
The ` + "`--print-keys`" + ` option cannot be combined with ` + "`--direct`" + ` or
` + "`--show-command`" + `.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-server/

`

const usageSSHExamples = `
Connect to machine 0:

    juju ssh 0

Connect to machine 1 and run command 'uname -a':

    juju ssh 1 uname -a

Connect to the leader mysql unit:

    juju ssh mysql/leader

Connect to a specific mysql unit:

    juju ssh mysql/0

Print the controller and virtual host key fingerprints for a mysql unit:

    juju ssh --print-keys mysql/0

Connect directly to a machine-backed unit as a different remote user:

    juju ssh --direct jenkins@jenkins/0

Select a private key for the jump server:

    juju ssh --ssh-key ~/.ssh/my_private_key mysql/0 echo hello

When using the Juju snap, the u2f-devices interface must be
connected to allow access to FIDO/U2F security keys:

    sudo snap connect juju:u2f-devices

Select a FIDO/U2F security key (e.g. YubiKey) for the jump server:

    juju ssh --ssh-key ~/.ssh/id_ed25519_sk mysql/0

Connect to a k8s unit targeting the charm container (the default):

	juju ssh snappass/0
	juju ssh --container charm snappass/0

Connect to a k8s unit targeting the redis container:

	juju ssh --container redis snappass/0

Interact with the Pebble instance in the workload container via the charm container:

    juju ssh snappass/0 PEBBLE_SOCKET=/charm/containers/redis/pebble.socket /charm/bin/pebble plan

**For the Kubernetes controller model in direct mode only:**

Connect to controller pod 0 in the api-server or charm container:

    juju ssh --direct 0
    juju ssh --direct --container charm 0
`

const (
	// SSHRetryDelay is the time to wait for an SSH connection to be established
	// to a single endpoint of a target.
	SSHRetryDelay = 2 * time.Second

	// SSHTimeout is the time to wait for all SSH negotiation and authentication process.
	SSHTimeout = 20 * time.Second
)

func NewSSHCommand(
	hostChecker jujussh.ReachableChecker,
	isTerminal func(any) bool,
	retryStrategy retry.CallArgs,
	publicKeyRetryStrategy retry.CallArgs,
) cmd.Command {
	c := &sshCommand{
		hostChecker:            hostChecker,
		isTerminal:             isTerminal,
		retryStrategy:          retryStrategy,
		publicKeyRetryStrategy: publicKeyRetryStrategy,
	}
	return modelcmd.Wrap(c)
}

var DefaultSSHRetryStrategy = retry.CallArgs{
	Clock:       clock.WallClock,
	MaxDuration: SSHTimeout,
	Delay:       SSHRetryDelay,
}

var DefaultSSHPublicKeyRetryStrategy = retry.CallArgs{
	Clock:       clock.WallClock,
	MaxDelay:    60 * time.Second,
	Delay:       1 * time.Second,
	Attempts:    10,
	BackoffFunc: retry.DoubleDelay,
}

// sshCommand is responsible for launching a ssh shell on a given unit or machine.
type sshCommand struct {
	modelType model.ModelType
	modelcmd.ModelCommandBase

	sshMachine
	sshContainer
	sshJump

	provider sshProvider

	hostChecker jujussh.ReachableChecker
	isTerminal  func(any) bool
	pty         autoBoolValue
	jump        bool
	direct      bool
	printKeys   bool

	retryStrategy          retry.CallArgs
	publicKeyRetryStrategy retry.CallArgs
}

func (c *sshCommand) SetFlags(f *gnuflag.FlagSet) {
	c.sshMachine.SetFlags(f)
	c.sshContainer.SetFlags(f)
	f.Var(&c.pty, "pty", "Enable pseudo-tty allocation")
	c.sshJump.SetFlags(f)
	f.BoolVar(&c.direct, "direct", false, "Use the legacy machine SSH or Kubernetes API route instead of the SSH jump server")
	f.BoolVar(&c.printKeys, "print-keys", false, "Print SHA256 fingerprints of the controller and virtual host keys")
}

func (c *sshCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "ssh",
		Args:     "<[user@]target> [openssh options] [command]",
		Purpose:  usageSSHSummary,
		Doc:      usageSSHDetails,
		Examples: usageSSHExamples,
		SeeAlso: []string{
			"scp",
		},
	})
}

func (c *sshCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("no target name specified")
	}
	if c.proxy.b != nil && *c.proxy.b && !c.direct {
		return errors.New("--proxy requires --direct")
	}
	if c.modelType, err = c.ModelType(context.TODO()); err != nil {
		return err
	}

	// Use jump by default, unless --direct is specified.
	c.jump = !c.direct

	if c.sshJump.showCommand && !c.jump {
		return errors.New("--show-command cannot be used with --direct")
	}
	if c.printKeys && c.direct {
		return errors.New("--print-keys cannot be used with --direct")
	}
	if c.printKeys && c.sshJump.showCommand {
		return errors.New("--print-keys cannot be used with --show-command")
	}
	// The jump provider is transparent to the model type. The container flag is
	// registered by the embedded sshContainer, so propagate its value to the
	// jump provider.
	if c.jump {
		c.provider = &c.sshJump
		c.sshJump.container = c.sshContainer.container
		c.sshJump.noHostKeyChecks = c.sshMachine.noHostKeyChecks
	} else if c.modelType == model.CAAS {
		c.provider = &c.sshContainer
	} else {
		c.provider = &c.sshMachine
	}
	c.provider.setTarget(args[0])
	c.provider.setArgs(args[1:])
	c.provider.setHostChecker(c.hostChecker)
	c.provider.setRetryStrategy(c.retryStrategy)
	c.provider.setPublicKeyRetryStrategy(c.publicKeyRetryStrategy)
	return nil
}

// ModelCommand defines methods of the model command.
type ModelCommand interface {
	NewControllerAPIRoot(ctx context.Context) (api.Connection, error)
	ModelDetails(ctx context.Context) (string, *jujuclient.ModelDetails, error)
	NewAPIRoot(ctx context.Context) (api.Connection, error)
	NewAPIClient(ctx context.Context) (*client.Client, error)
	ModelIdentifier() (string, error)
	ControllerDetails() (*jujuclient.ControllerDetails, error)
	CurrentAccountDetails() (*jujuclient.AccountDetails, error)
}

// sshProvider is implemented by either a CaaS or IaaS model instance.
type sshProvider interface {
	initRun(context.Context, ModelCommand) error
	cleanupRun()
	setLeaderAPI(ctx context.Context, leaderAPI LeaderAPI)
	setHostChecker(checker jujussh.ReachableChecker)
	resolveTarget(context.Context, string) (*resolvedTarget, error)
	maybeResolveLeaderUnit(context.Context, string) (string, error)
	ssh(ctx Context, enablePty bool, target *resolvedTarget) error
	copy(Context) error

	getTarget() string
	setTarget(target string)

	getArgs() []string
	setArgs(Args []string)

	setRetryStrategy(retry.CallArgs)
	setPublicKeyRetryStrategy(retry.CallArgs)
}

// Run resolves the given target to a machine or unit, then opens
// an SSH connection to this target.
func (c *sshCommand) Run(ctx *cmd.Context) error {
	if err := c.provider.initRun(ctx.Context, &c.ModelCommandBase); err != nil {
		return errors.Trace(err)
	}
	defer c.provider.cleanupRun()

	if c.printKeys {
		return c.sshJump.printHostKeys(ctx, ctx.Stdout)
	}

	target, err := c.provider.resolveTarget(ctx, c.provider.getTarget())
	if err != nil {
		return err
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

func (b *autoBoolValue) Get() any {
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

// LeaderAPI is implemented by types that can query for a Leader based on
// application name.
type LeaderAPI interface {
	Leader(context.Context, string) (string, error)
	Close() error
}

type leaderResolver struct {
	leaderAPI      LeaderAPI
	resolvedLeader string
}

func (c *leaderResolver) maybeResolveLeaderUnit(ctx context.Context, target string) (string, error) {
	if !strings.HasSuffix(target, "/leader") {
		return target, nil
	}
	if c.resolvedLeader != "" {
		return c.resolvedLeader, nil
	}

	app := strings.Split(target, "/")[0]

	// Do not call leaderAPI.Close() here, it's used again
	// upstream from here.
	var err error
	c.resolvedLeader, err = c.leaderAPI.Leader(ctx, app)
	return c.resolvedLeader, errors.Trace(err)
}

func isTerminal(f any) bool {
	f_, ok := f.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f_.Fd())
}
