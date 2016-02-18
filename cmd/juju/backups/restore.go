// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
)

func newRestoreCommand() cmd.Command {
	return modelcmd.Wrap(&restoreCommand{})
}

// restoreCommand is a subcommand of backups that implement the restore behavior
// it is invoked with "juju restore-backup".
type restoreCommand struct {
	CommandBase
	constraints constraints.Value
	filename    string
	backupId    string
	bootstrap   bool
	uploadTools bool
}

var restoreDoc = `
Restores a backup that was previously created with "juju create-backup".

This command creates a new controller and arranges for it to replace
the previous controller for a model.  It does *not* restore
an existing server to a previous state, but instead creates a new server
with equivalent state.  As part of restore, all known instances are
configured to treat the new controller as their master.

The given constraints will be used to choose the new instance.

If the provided state cannot be restored, this command will fail with
an appropriate message.  For instance, if the existing bootstrap
instance is already running then the command will fail with a message
to that effect.
`

// Info returns the content for --help.
func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "restore",
		Purpose: "restore from a backup archive to a new controller",
		Args:    "",
		Doc:     strings.TrimSpace(restoreDoc),
	}
}

// SetFlags handles known option flags.
func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.Var(constraints.ConstraintsValue{Target: &c.constraints},
		"constraints", "set model constraints")

	f.BoolVar(&c.bootstrap, "b", false, "bootstrap a new state machine")
	f.StringVar(&c.filename, "file", "", "provide a file to be used as the backup.")
	f.StringVar(&c.backupId, "id", "", "provide the name of the backup to be restored.")
	f.BoolVar(&c.uploadTools, "upload-tools", false, "upload tools if bootstraping a new machine.")
}

// Init is where the preconditions for this commands can be checked.
func (c *restoreCommand) Init(args []string) error {
	if c.filename == "" && c.backupId == "" {
		return errors.Errorf("you must specify either a file or a backup id.")
	}
	if c.filename != "" && c.backupId != "" {
		return errors.Errorf("you must specify either a file or a backup id but not both.")
	}
	if c.backupId != "" && c.bootstrap {
		return errors.Errorf("it is not possible to rebootstrap and restore from an id.")
	}
	var err error
	if c.filename != "" {
		c.filename, err = filepath.Abs(c.filename)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// runRestore will implement the actual calls to the different Client parts
// of restore.
func (c *restoreCommand) runRestore(ctx *cmd.Context) error {
	client, closer, err := c.newClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()
	var target string
	var rErr error
	if c.filename != "" {
		target = c.filename
		archive, meta, err := getArchive(c.filename)
		if err != nil {
			return errors.Trace(err)
		}
		defer archive.Close()

		rErr = client.RestoreReader(archive, meta, c.newClient)
	} else {
		target = c.backupId
		rErr = client.Restore(c.backupId, c.newClient)
	}
	if rErr != nil {
		return errors.Trace(rErr)
	}

	fmt.Fprintf(ctx.Stdout, "restore from %q completed\n", target)
	return nil
}

// rebootstrap will bootstrap a new server in safe-mode (not killing any other agent)
// if there is no current server available to restore to.
func (c *restoreCommand) rebootstrap(ctx *cmd.Context) error {

	// TODO(axw) delete this and -b in 2.0-beta2. We will update bootstrap
	// with a flag to specify a restore file. When we do that, we'll need
	// to extract the CA cert from the backup, and we'll need to reset the
	// password after restore so the admin user can login.
	controllerName := c.ControllerName()
	legacyStore, err := configstore.Default()
	if err != nil {
		return errors.Trace(err)
	}
	info, err := legacyStore.ReadInfo(configstore.EnvironInfoName(
		controllerName, configstore.AdminModelName(controllerName),
	))
	if err != nil {
		return errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
	if err != nil {
		return errors.Trace(err)
	}

	// Turn on safe mode so that the newly bootstrapped instance
	// will not destroy all the instances it does not know about.
	cfg, err = cfg.Apply(map[string]interface{}{
		"provisioner-safe-mode": true,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot enable provisioner-safe-mode")
	}
	env, err := environs.New(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	instanceIds, err := env.ControllerInstances()
	if err != nil {
		return errors.Annotatef(err, "cannot determine controller instances")
	}
	if len(instanceIds) == 0 {
		return errors.Errorf("no instances found; perhaps the model was not bootstrapped")
	}
	inst, err := env.Instances(instanceIds)
	if err == nil {
		return errors.Errorf("old bootstrap instance %q still seems to exist; will not replace", inst)
	}
	if err != environs.ErrNoInstances {
		return errors.Annotatef(err, "cannot detect whether old instance is still running")
	}

	cons := c.constraints
	args := bootstrap.BootstrapParams{EnvironConstraints: cons, UploadTools: c.uploadTools}
	if err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, args); err != nil {
		return errors.Annotatef(err, "cannot bootstrap new instance")
	}
	return nil
}

func (c *restoreCommand) newClient() (*backups.Client, func() error, error) {
	client, err := c.NewAPIClient()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	backupsClient, ok := client.(*backups.Client)
	if !ok {
		return nil, nil, errors.Errorf("invalid client for backups")
	}
	return backupsClient, client.Close, nil
}

// Run is the entry point for this command.
func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}
	if c.bootstrap {
		if err := c.rebootstrap(ctx); err != nil {
			return errors.Trace(err)
		}
	}
	return c.runRestore(ctx)
}
