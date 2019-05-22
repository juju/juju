// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"path/filepath"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/winrm"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/environs/manual/winrmprovisioner"
	"github.com/juju/juju/juju/osenv"
)

// NewRemoveManualCommand returns a command used to remove a specified machine.
func NewRemoveManualCommand() cmd.Command {
	return modelcmd.Wrap(&removeManualCommand{})
}

// removeManualCommand causes an existing machine to be destroyed.
type removeManualCommand struct {
	baseMachinesCommand
	fs        *gnuflag.FlagSet
	Placement *instance.Placement
}

const removeManualMachineDoc = `
Removing manual provisioned machines can be cleaned up as long as they're still
accessible via SSH. Clean up of the machine removes the Juju services along with
cleaning up of lib and log directories.

Examples:

    juju remove-manual-machine ssh:user@10.10.0.3
    juju remove-manual-machine winrm:user@10.10.0.3

See also:
	add-machine
	remove-machine
`

// Info implements Command.Info.
func (c *removeManualCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-manual-machine",
		Args:    "[ssh:[user@]host | winrm:[user@]host]",
		Purpose: "Removes a manual machine.",
		Doc:     removeManualMachineDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeManualCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.fs = f
}

func (c *removeManualCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("wrong number of arguments, expected 1")
	}

	placement, err := instance.ParsePlacement(args[0])
	if err != nil {
		return errors.Annotatef(err, "placement parse error for %q", args[0])
	}
	if placement.Scope == instance.MachineScope {
		return errors.Errorf("remove-manual-machine expects user@host argument. Instead please use remove-machine %s", args[0])
	}
	if placement.Directive == "" {
		return errors.Errorf("invalid placement directive %q", args[0])
	}
	c.Placement = placement

	return nil
}

// Run implements Command.Run.
func (c *removeManualCommand) Run(ctx *cmd.Context) error {
	err := c.removeManual(ctx)
	if err == errNonManualScope {
		return errors.Errorf("unexpected placement scope %s", c.Placement.Scope)
	}
	if err != nil {
		return err
	}
	return nil
}

var (
	sshRemover   = sshprovisioner.RemoveMachine
	winrmRemover = winrmprovisioner.RemoveMachine
)

func (c *removeManualCommand) removeManual(ctx *cmd.Context) error {
	user, host := splitUserHost(c.Placement.Directive)

	var removeMachine manual.RemoveMachineFunc
	var removeMachineCommandExec manual.CommandExec
	var removeMachineWinrmClient manual.WinrmClientAPI
	switch c.Placement.Scope {
	case sshScope:
		removeMachine = sshRemover
		removeMachineCommandExec = sshprovisioner.DefaultCommandExec()
	case winrmScope:
		removeMachine = winrmRemover
		var err error
		removeMachineWinrmClient, err = c.winrmClient(user, host)
		if err != nil {
			return errors.Trace(err)
		}
	default:
		return errNonManualScope
	}

	args := manual.RemoveMachineArgs{
		Host:           host,
		User:           user,
		Stdin:          ctx.Stdin,
		Stdout:         ctx.Stdout,
		Stderr:         ctx.Stderr,
		CommandExec:    removeMachineCommandExec,
		WinrmClientAPI: removeMachineWinrmClient,
	}

	if err := removeMachine(args); err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("machine removed")
	return nil
}

func (c *removeManualCommand) winrmClient(user, host string) (manual.WinrmClientAPI, error) {
	base := osenv.JujuXDGDataHomePath("x509")
	keyPath := filepath.Join(base, "winrmkey.pem")
	certPath := filepath.Join(base, "winrmcert.crt")
	cert := winrm.NewX509()
	if err := cert.LoadClientCert(keyPath, certPath); err != nil {
		return nil, errors.Annotatef(err, "connot load/create x509 client certs for winrm connection")
	}
	if err := cert.LoadCACert(filepath.Join(base, "winrmcacert.crt")); err != nil {
		logger.Infof("cannot not find any CA cert to load")
	}

	cfg := winrm.ClientConfig{
		User:    user,
		Host:    host,
		Key:     cert.ClientKey(),
		Cert:    cert.ClientCert(),
		Timeout: 25 * time.Second,
		Secure:  true,
	}

	caCert := cert.CACert()
	if caCert == nil {
		logger.Infof("Skipping winrm CA validation")
		cfg.Insecure = true
	} else {
		cfg.CACert = caCert
	}

	client, err := winrm.NewClient(cfg)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create WinRM client connection")
	}
	return client, nil
}
