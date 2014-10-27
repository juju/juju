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

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/utils/ssh"
)

type RestoreCommand struct {
	CommandBase
	constraints constraints.Value
	filename    string
	backupId    string
	bootstrap   bool
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
	f.Var(constraints.ConstraintsValue{Target: &c.constraints},
		"constraints", "set environment constraints")

	f.BoolVar(&c.bootstrap, "b", false, "bootstrap a new state machine")
	f.StringVar(&c.filename, "file", "", "provide a file to be used as the backup.")
	f.StringVar(&c.backupId, "name", "", "provide the name of the backup to be restored.")

}

func (c *RestoreCommand) Init(args []string) error {
	if c.filename == "" && c.backupId == "" {
		return errors.Errorf("you must specify either a file or a backup name.")
	}
	return nil
}

const restoreAPIIncompatibility = "server version not compatible for " +
	"restore with client version"

func (c *RestoreCommand) runRestore(ctx *cmd.Context, client APIClient) error {

	fileName := filepath.Base(c.filename)
	if err := client.Restore(fileName, c.backupId); err != nil {

		if params.IsCodeNotImplemented(err) {
			return errors.Errorf(restoreAPIIncompatibility)
		}
		if err != rpc.ErrShutdown {
			return errors.Trace(err)
		}
		client, err = c.NewAPIClient()
		if err != nil {
			return errors.Trace(err)
		}

	}
	if err := client.FinishRestore(); err != nil {
		if params.IsCodeNotImplemented(err) {
			return errors.Errorf(restoreAPIIncompatibility)
		}
		return errors.Trace(err)
	}
	fmt.Fprintf(ctx.Stdout, "restore from %s completed\n", c.filename)
	return nil
}

func (c *RestoreCommand) rebootstrap(ctx *cmd.Context) (environs.Environ, error) {
	cons := c.constraints
	store, err := configstore.Default()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := c.Config(store)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Turn on safe mode so that the newly bootstrapped instance
	// will not destroy all the instances it does not know about.
	cfg, err = cfg.Apply(map[string]interface{}{
		"provisioner-safe-mode": true,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot enable provisionar-safe-mode")
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instanceIds, err := env.StateServerInstances()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot determine state server instances")
	}
	if len(instanceIds) == 0 {
		return nil, errors.Errorf("no instances found; perhaps the environment was not bootstrapped")
	}
	inst, err := env.Instances(instanceIds)
	if err == nil {
		return nil, errors.Errorf("old bootstrap instance %q still seems to exist; will not replace", inst)
	}
	if err != environs.ErrNoInstances {
		return nil, errors.Annotatef(err, "cannot detect whether old instance is still running")
	}

	args := bootstrap.BootstrapParams{Constraints: cons}
	if err := bootstrap.Bootstrap(ctx, env, args); err != nil {
		return nil, errors.Annotatef(err, "cannot bootstrap new instance")
	}
	return env, nil
}

func (c *RestoreCommand) doUpload(client APIClient) error {
	addr, err := client.PublicAddress("0")
	if err != nil {
		return errors.Trace(err)
	}

	fileName := filepath.Base(c.filename)

	if err := ssh.Copy([]string{c.filename, fmt.Sprintf("ubuntu@%s:%s", addr, fileName)}, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *RestoreCommand) Run(ctx *cmd.Context) error {
	if c.bootstrap {
		_, err := c.rebootstrap(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Empty string will get a client for current default
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}

	if err := client.PrepareRestore(); err != nil {
		if params.IsCodeNotImplemented(err) {
			return errors.Errorf(restoreAPIIncompatibility)
		}
		return errors.Trace(err)
	}

	defer client.Close()
	if c.filename != "" {
		if err := c.doUpload(client); err != nil {
			return errors.Annotatef(err, "cannot upload backup")
		}
	}

	return c.runRestore(ctx, client)
}
