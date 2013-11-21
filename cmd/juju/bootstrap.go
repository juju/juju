// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

const bootstrapDoc = `
bootstrap starts a new environment of the current type (it will return an error
if the environment has already been bootstrapped).  Bootstrapping an environment
will provision a new machine in the environment and run the juju state server on
that machine.

If constraints are specified in the bootstrap command, they will apply to the 
machine provisioned for the juju state server.  They will also be set as default
constraints on the environment for all future machines, exactly as if the
constraints were set with juju set-constraints.

Because bootstrap starts a machine in the cloud environment asynchronously, the
command will likely return before the state server is fully running.  Time for
bootstrap to be complete varies across cloud providers from a small number of
seconds to several minutes.  Most other commands are synchronous and will wait
until bootstrap is finished to complete.

See Also:
   juju help switch
   juju help constraints
   juju help set-constraints
`

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	cmd.EnvCommandBase
	Constraints constraints.Value
	UploadTools bool
	Series      []string
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap",
		Purpose: "start up an environment from scratch",
		Doc:     bootstrapDoc,
	}
}

func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "set environment constraints")
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
	f.Var(seriesVar{&c.Series}, "series", "upload tools for supplied comma-separated series list")
}

func (c *BootstrapCommand) Init(args []string) error {
	if len(c.Series) > 0 && !c.UploadTools {
		return fmt.Errorf("--series requires --upload-tools")
	}
	return cmd.CheckEmpty(args)
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *BootstrapCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	environ, err := environs.PrepareFromName(c.EnvName, store)
	if err != nil {
		return err
	}
	// If the environment has a special bootstrap Storage, use it wherever
	// we'd otherwise use environ.Storage.
	if bs, ok := environ.(environs.BootstrapStorager); ok {
		if err := bs.EnableBootstrapStorage(); err != nil {
			return fmt.Errorf("failed to enable bootstrap storage: %v", err)
		}
	}
	if err := bootstrap.EnsureNotBootstrapped(environ); err != nil {
		return err
	}

	// TODO (wallyworld): 2013-09-20 bug 1227931
	// We can set a custom tools data source instead of doing an
	// unecessary upload.
	if environ.Config().Type() == provider.Local {
		c.UploadTools = true
	}
	if c.UploadTools {
		err = c.uploadTools(environ)
		if err != nil {
			return err
		}
	}

	// Bootstrap is synchronous, and will spawn a subprocess
	// to complete the procedure. If the user hits Ctrl-C,
	// SIGINT is sent to the foreground process attached to
	// the terminal, which will be the subprocess at that
	// point. When that exits (with an error status), juju
	// will attempt to clean up.
	//
	// Ignore SIGINT signals, to prevent double Ctrl-C
	// from killing the cleanup from a cancelled bootstrap.
	signal.Notify(make(chan os.Signal), os.Interrupt)
	return bootstrap.Bootstrap(environ, c.Constraints)
}

func (c *BootstrapCommand) uploadTools(environ environs.Environ) error {
	// Force version.Current, for consistency with subsequent upgrade-juju
	// (see UpgradeJujuCommand).
	forceVersion := uploadVersion(version.Current.Number, nil)
	cfg := environ.Config()
	series := getUploadSeries(cfg, c.Series)
	agenttools, err := sync.Upload(environ.Storage(), &forceVersion, series...)
	if err != nil {
		return err
	}
	cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": agenttools.Version.Number.String(),
	})
	if err == nil {
		err = environ.SetConfig(cfg)
	}
	if err != nil {
		return fmt.Errorf("failed to update environment configuration: %v", err)
	}
	return nil
}

type seriesVar struct {
	target *[]string
}

func (v seriesVar) Set(value string) error {
	names := strings.Split(value, ",")
	for _, name := range names {
		if !charm.IsValidSeries(name) {
			return fmt.Errorf("invalid series name %q", name)
		}
	}
	*v.target = names
	return nil
}

func (v seriesVar) String() string {
	return strings.Join(*v.target, ",")
}

// getUploadSeries returns the supplied series with duplicates removed if
// non-empty; otherwise it returns a default list of series we should
// probably upload, based on cfg.
func getUploadSeries(cfg *config.Config, series []string) []string {
	unique := set.NewStrings(series...)
	if unique.IsEmpty() {
		unique.Add(version.Current.Series)
		unique.Add(config.DefaultSeries)
		unique.Add(cfg.DefaultSeries())
	}
	return unique.Values()
}
