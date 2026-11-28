// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/retry"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	jujussh "github.com/juju/juju/internal/network/ssh"
)

var usageSCPSummary = `
Securely transfer files within a model.`[1:]

var usageSCPDetails = `
By default, this command copies through the controller's SSH jump server.
Use a local path or a remote path of the form ` + "`<target>:<path>`" + `. Choose the
target from ` + "`juju status`" + `; ` + "`<application>/leader`" + ` selects the leader unit.

### Machine models

Use a top-level machine number or a machine-backed unit. The client runs
OpenSSH scp; specify one or more sources followed by the destination.
The machine login is ` + "`ubuntu`" + ` on the jump route. To pass options to scp,
put them after ` + "`--`" + `, before the paths. Use ` + "`-r`" + ` for a directory, ` + "`-C`" + `
for compression or ` + "`-3`" + ` to copy between two remote machines through your
client. By default the client verifies both jump and virtual host keys. Use
` + "`--show-command`" + ` to print the OpenSSH command for a machine copy instead
of running it. The printed command uses OpenSSH's own host-key configuration;
compare trust prompts with ` + "`juju ssh --print-keys <target>`" + `.

### Kubernetes models

Specify exactly one local path and one path in a unit, in either order.
The jump route streams an archive and requires ` + "`tar`" + ` in the selected
container. The default container is ` + "`charm`" + `; use ` + "`--container`" + ` to choose
another in the same pod. Pod-to-pod copies, scp options and
` + "`--show-command`" + ` are not supported on this route.

### Direct mode

For an older controller without jump support, specify ` + "`--direct`" + `; the
client does not switch automatically. A machine copy then uses ordinary SSH,
where ` + "`user@target:path`" + ` can select a different account. A Kubernetes copy
uses the Kubernetes API. Neither direct route supports ` + "`--show-command`" + `.

On machine models, ` + "`--direct --proxy`" + ` copies through the controller
machine's OpenSSH server (port 22 by default). If you omit ` + "`--proxy`" + ` in
direct mode, Juju uses the model's ` + "`proxy-ssh`" + ` setting. To bypass the
proxy when your client can reach the machine directly, use ` + "`--direct --proxy=false`" + `.

On client build ` + "`4.1-beta3-048d091`" + ` the proxy hop fails, and
` + "`--proxy=false`" + ` does not override a model with ` + "`proxy-ssh=true`" + `.
Use the default jump route (omit both flags) if the controller supports it.
If you need the proxy hop, use a client with the fix for Juju issue #23913.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-server/

`

const usageSCPExamples = `
Copy the config of a Charmed Kubernetes cluster to ` + "`~/.kube/config`" + `:

    juju scp kubernetes-master/0:config ~/.kube/config

Copy file ` + "`/var/log/syslog`" + ` from machine ` + "`2`" + ` to the client's
current working directory:

    juju scp 2:/var/log/syslog .

Recursively copy the ` + "`/var/log/mongodb`" + ` directory from the
` + "`mongodb/0`" + ` unit to the client's local ` + "`remote-logs`" + ` directory:

    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs

Copy ` + "`foo.txt`" + ` to the ` + "`apache2/1`" + ` machine unit in model ` + "`prod`" + ` over the
default jump route. Pass ` + "`-C`" + ` to scp to enable compression:

    juju scp -m prod -- -C foo.txt apache2/1:

Copy multiple files from the client's current working directory to
the ` + "`/home/ubuntu`" + ` directory of machine ` + "`2`" + `:

    juju scp file1 file2 2:

In direct mode, download a file from machine ` + "`3`" + ` as remote user ` + "`bob`" + `:

    juju scp --direct bob@3:/home/bob/report.txt .

Copy ` + "`file.dat`" + ` from machine 0 to the machine hosting unit ` + "`foo/0`" + `
(` + "`-- -3`" + `):

    juju scp -- -3 0:file.dat foo/0:

Copy a file (` + "`chunks-inspect`" + `) from ` + "`localhost`" + ` to the ` + "`/loki`" + ` directory
in a specific container in a Juju unit running in Kubernetes:

    juju scp --container loki chunks-inspect loki-k8s/0:/loki
`

func NewSCPCommand(hostChecker jujussh.ReachableChecker, retryStrategy retry.CallArgs, publicKeyRetryStrategy retry.CallArgs) cmd.Command {
	c := new(scpCommand)
	c.hostChecker = hostChecker
	c.retryStrategy = retryStrategy
	c.publicKeyRetryStrategy = publicKeyRetryStrategy
	return modelcmd.Wrap(c)
}

// scpCommand is responsible for launching a scp command to copy files to/from remote machine(s)
type scpCommand struct {
	modelcmd.ModelCommandBase

	modelType model.ModelType

	sshMachine
	sshContainer
	sshJump

	provider sshProvider

	hostChecker jujussh.ReachableChecker
	jump        bool
	direct      bool

	retryStrategy          retry.CallArgs
	publicKeyRetryStrategy retry.CallArgs
}

func (c *scpCommand) SetFlags(f *gnuflag.FlagSet) {
	c.sshMachine.SetFlags(f)
	c.sshContainer.SetFlags(f)
	c.sshJump.SetFlags(f)
	f.BoolVar(&c.direct, "direct", false, "Use the legacy machine SSH or Kubernetes API route instead of the SSH jump server")
}

func (c *scpCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "scp",
		Args:     "<source> <destination>",
		Purpose:  usageSCPSummary,
		Doc:      usageSCPDetails,
		Examples: usageSCPExamples,
		SeeAlso: []string{
			"ssh",
		},
	})
}

func (c *scpCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.Errorf("at least two arguments required")
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
	if c.jump {
		c.provider = &c.sshJump
		c.sshJump.noHostKeyChecks = c.sshMachine.noHostKeyChecks
		if c.modelType == model.CAAS {
			c.sshJump.container = c.sshContainer.container
		}
	} else if c.modelType == model.CAAS {
		c.provider = &c.sshContainer
	} else {
		c.provider = &c.sshMachine
	}

	c.provider.setArgs(args)
	c.provider.setHostChecker(c.hostChecker)
	c.provider.setRetryStrategy(c.retryStrategy)
	c.provider.setPublicKeyRetryStrategy(c.publicKeyRetryStrategy)
	return nil
}

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
func (c *scpCommand) Run(ctx *cmd.Context) error {
	if err := c.provider.initRun(ctx.Context, &c.ModelCommandBase); err != nil {
		return errors.Trace(err)
	}
	defer c.provider.cleanupRun()
	return c.provider.copy(ctx)
}
