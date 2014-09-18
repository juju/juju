// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/utils/ssh"
)

type restoreClient interface {
	Restore(string, bool) error
}

type RestoreCommand struct {
	CommandBase
	Constraints constraints.Value
	Filename    string
	Upload      bool
	Bootstrap   bool
}

var restoreDoc = `
Restores a backup that was previously created with "juju backup".

This command creates a new state server and arranges for it to replace
the previous state server for an environment.  It does *not* restore
an existing server to a previous state, but instead creates a new server
with equivanlent state.  As part of restore, all known instances are
configured to treat the new state server as their master.

The given constraints will be used to choose the new instance.

If the provided state cannot be restored, this command will fail with
an appropriate message.  For instance, if the existing bootstrap
instance is already running then the command will fail with a message
to that effect.
`

func (c *RestoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "restore",
		Purpose: "restore a state server backup made with juju backup",
		Args:    "[-u] [-b] <backupfile.tar.gz>",
		Doc:     strings.TrimSpace(restoreDoc),
	}
}

func (c *RestoreCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints},
		"constraints", "set environment constraints")

	f.BoolVar(&c.Upload, "u", false, "upload the file")
	f.BoolVar(&c.Bootstrap, "b", false, "bootstrap a new state machine")
}

func (c *RestoreCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no backup name specified")
	}
	c.Filename = args[0]
	return nil
}

const restoreAPIIncompatibility = "server version not compatible for " +
	"restore with client version"

func (c *RestoreCommand) runRestore(ctx *cmd.Context, client restoreClient) error {

	fileName := filepath.Base(c.Filename)

	if err := client.Restore(fileName, c.Upload); err != nil {

		logger.Debugf("------------> Called Restore")
		if params.IsCodeNotImplemented(err) {
			return errors.Errorf(restoreAPIIncompatibility)
		}
		return err
	}
	fmt.Fprintf(ctx.Stdout, "restore from %s completed\n", c.Filename)
	return nil
}

func (c *RestoreCommand) rebootstrap(ctx *cmd.Context) (environs.Environ, error) {
	cons := c.Constraints
	store, err := configstore.Default()
	if err != nil {
		return nil, err
	}
	cfg, err := c.Config(store)
	if err != nil {
		return nil, err
	}
	// Turn on safe mode so that the newly bootstrapped instance
	// will not destroy all the instances it does not know about.
	cfg, err = cfg.Apply(map[string]interface{}{
		"provisioner-safe-mode": true,
	})
	if err != nil {
		return nil, errors.Errorf("cannot enable provisioner-safe-mode: %v", err)
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	instanceIds, err := env.StateServerInstances()
	if err != nil {
		return nil, errors.Errorf("cannot determine state server instances: %v", err)
	}
	if len(instanceIds) == 0 {
		return nil, errors.Errorf("no instances found; perhaps the environment was not bootstrapped")
	}
	if len(instanceIds) > 1 {
		return nil, errors.Errorf("restore does not support HA juju configurations yet")
	}
	inst, err := env.Instances(instanceIds)
	if err == nil {
		return nil, errors.Errorf("old bootstrap instance %q still seems to exist; will not replace", inst)
	}
	if err != environs.ErrNoInstances {
		return nil, errors.Errorf("cannot detect whether old instance is still running: %v", err)
	}
	// Remove the storage so that we can bootstrap without the provider complaining.
	if err := env.Storage().Remove(common.StateFile); err != nil {
		return nil, errors.Errorf("cannot remove %q from storage: %v", common.StateFile, err)
	}

	// TODO If we fail beyond here, then we won't have a state file and
	// we won't be able to re-run this script because it fails without it.
	// We could either try to recreate the file if we fail (which is itself
	// error-prone) or we could provide a --no-check flag to make
	// it go ahead anyway without the check.

	args := bootstrap.BootstrapParams{Constraints: cons}
	if err := bootstrap.Bootstrap(ctx, env, args); err != nil {
		return nil, errors.Errorf("cannot bootstrap new instance: %v", err)
	}
	return env, nil
}

func (c *RestoreCommand) doUpload(client *api.Client) error {
	// The
	addr, err := client.PublicAddress("0")
	if err != nil {
		return err
	}

	fileName := filepath.Base(c.Filename)

	if err := ssh.Copy([]string{c.Filename, fmt.Sprintf("ubuntu@%s:%s", addr, fileName)}, nil); err != nil {
		return err
	}
	//TODO(perrito666) add to envstorage, is it worthy? or will I need to remove afer?
	// Also make sure to have ensurebackups
	return nil
}

func (c *RestoreCommand) Run(ctx *cmd.Context) error {
	if c.Bootstrap {
		_, err := c.rebootstrap(ctx)
		if err != nil {
			return err
		}
	}

	logger.Debugf("------------> bootstrapped")
	// Empty string will get a client for current default
	client, err := juju.NewAPIClientFromName("")
	if err != nil {
		return err
	}

	logger.Debugf("------------> have a client")
	defer client.Close()

	if c.Upload {
		c.doUpload(client)
	}
	logger.Debugf("------------> uploaded")

	return c.runRestore(ctx, client)
}
