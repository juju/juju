// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"crypto/rand"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/jujuclient"
)

// NewRestoreCommand returns a command used to restore a backup.
func NewRestoreCommand() cmd.Command {
	restoreCmd := &restoreCommand{}
	restoreCmd.getEnvironFunc = restoreCmd.getEnviron
	restoreCmd.newAPIClientFunc = func() (RestoreAPI, error) {
		return restoreCmd.newClient()
	}
	restoreCmd.getArchiveFunc = getArchive
	restoreCmd.waitForAgentFunc = common.WaitForAgentInitialisation
	return modelcmd.Wrap(restoreCmd)
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

	newAPIClientFunc func() (RestoreAPI, error)
	getEnvironFunc   func(string, *params.BackupsMetadataResult) (environs.Environ, error)
	getArchiveFunc   func(string) (ArchiveReader, *params.BackupsMetadataResult, error)
	waitForAgentFunc func(ctx *cmd.Context, c *modelcmd.ModelCommandBase, controllerName string) error
}

// RestoreAPI is used to invoke various API calls.
type RestoreAPI interface {
	// Close is taken from io.Closer.
	Close() error

	// Restore is taken from backups.Client.
	Restore(backupId string, newClient backups.ClientConnection) error

	// RestoreReader is taken from backups.Client.
	RestoreReader(r io.ReadSeeker, meta *params.BackupsMetadataResult, newClient backups.ClientConnection) error
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

var BootstrapFunc = bootstrap.Bootstrap

// Info returns the content for --help.
func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "restore-backup",
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

// getEnviron returns the environ for the specified controller, or
// mocked out environ for testing.
func (c *restoreCommand) getEnviron(controllerName string, meta *params.BackupsMetadataResult) (environs.Environ, error) {
	// TODO(axw) delete this and -b in 2.0-beta2. We will update bootstrap
	// with a flag to specify a restore file. When we do that, we'll need
	// to extract the CA cert from the backup, and we'll need to reset the
	// password after restore so the admin user can login.
	// We also need to store things like the admin-secret, controller
	// certificate etc with the backup.
	store := c.ClientStore()
	cfg, err := modelcmd.NewGetBootstrapConfigFunc(store)(controllerName)
	if err != nil {
		return nil, errors.Annotate(err, "cannot restore from a machine other than the one used to bootstrap")
	}

	// Reset current model to admin so first bootstrap succeeds.
	err = store.SetCurrentModel(controllerName, environs.AdminUser, "admin")
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We may have previous controller metadata. We need to update that so it
	// will contain the new CA Cert and UUID required to connect to the newly
	// bootstrapped controller API.
	details := jujuclient.ControllerDetails{
		ControllerUUID: cfg.ControllerUUID(),
		CACert:         meta.CACert,
	}
	err = store.UpdateController(controllerName, details)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get the local admin user so we can use the password as the admin secret.
	var adminSecret string
	account, err := store.AccountByName(controllerName, environs.AdminUser)
	if err == nil {
		adminSecret = account.Password
	} else if errors.IsNotFound(err) {
		// No relevant local admin user so generate a new secret.
		buf := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			return nil, errors.Annotate(err, "generating new admin secret")
		}
		adminSecret = fmt.Sprintf("%x", buf)
	} else {
		return nil, errors.Trace(err)
	}

	// Turn on safe mode so that the newly bootstrapped instance
	// will not destroy all the instances it does not know about.
	// Also set the admin secret and ca cert info.
	cfg, err = cfg.Apply(map[string]interface{}{
		"provisioner-safe-mode": true,
		"admin-secret":          adminSecret,
		"ca-private-key":        meta.CAPrivateKey,
		"ca-cert":               meta.CACert,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot enable provisioner-safe-mode")
	}
	return environs.New(cfg)
}

// rebootstrap will bootstrap a new server in safe-mode (not killing any other agent)
// if there is no current server available to restore to.
func (c *restoreCommand) rebootstrap(ctx *cmd.Context, meta *params.BackupsMetadataResult) error {
	env, err := c.getEnvironFunc(c.ControllerName(), meta)
	if err != nil {
		return errors.Trace(err)
	}
	instanceIds, err := env.ControllerInstances()
	if err != nil && errors.Cause(err) != environs.ErrNotBootstrapped {
		return errors.Annotatef(err, "cannot determine controller instances")
	}
	if len(instanceIds) > 0 {
		inst, err := env.Instances(instanceIds)
		if err == nil {
			return errors.Errorf("old bootstrap instance %q still seems to exist; will not replace", inst)
		}
		if err != environs.ErrNoInstances {
			return errors.Annotatef(err, "cannot detect whether old instance is still running")
		}
	}

	// We require a hosted model config to bootstrap. We'll fill in some defaults
	// just to get going. The restore will clear the initial state.
	hostedModelUUID, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	hostedModelConfig := map[string]interface{}{
		"name":         "default",
		config.UUIDKey: hostedModelUUID.String(),
	}

	args := bootstrap.BootstrapParams{
		ModelConstraints:  c.constraints,
		UploadTools:       c.uploadTools,
		BuildToolsTarball: sync.BuildToolsTarball,
		HostedModelConfig: hostedModelConfig,
	}
	if err := BootstrapFunc(modelcmd.BootstrapContext(ctx), env, args); err != nil {
		return errors.Annotatef(err, "cannot bootstrap new instance")
	}

	// New controller is bootstrapped, so now record the API address so
	// we can connect.
	err = common.SetBootstrapEndpointAddress(c.ClientStore(), c.ControllerName(), env)
	if err != nil {
		errors.Trace(err)
	}
	// To avoid race conditions when running scripted bootstraps, wait
	// for the controller's machine agent to be ready to accept commands
	// before exiting this bootstrap command.
	return c.waitForAgentFunc(ctx, &c.ModelCommandBase, c.ControllerName())
}

func (c *restoreCommand) newClient() (*backups.Client, error) {
	client, err := c.NewAPIClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backupsClient, ok := client.(*backups.Client)
	if !ok {
		return nil, errors.Errorf("invalid client for backups")
	}
	return backupsClient, nil
}

// Run is the entry point for this command.
func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}

	var archive ArchiveReader
	var meta *params.BackupsMetadataResult
	target := c.backupId
	if c.filename != "" {
		// Read archive specified by the filename;
		// we'll need the info later regardless if
		// we need it now to rebootstrap.
		target = c.filename
		var err error
		archive, meta, err = c.getArchiveFunc(c.filename)
		if err != nil {
			return errors.Trace(err)
		}
		defer archive.Close()

		if c.bootstrap {
			if err := c.rebootstrap(ctx, meta); err != nil {
				return errors.Trace(err)
			}
		}
	}

	client, err := c.newAPIClientFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	// We have a backup client, now use the relevant method
	// to restore the backup.
	if c.filename != "" {
		err = client.RestoreReader(archive, meta, c.newClient)
	} else {
		err = client.Restore(c.backupId, c.newClient)
	}
	if err != nil {
		return nil
	}
	fmt.Fprintf(ctx.Stdout, "restore from %q completed\n", target)
	return nil
}
