// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/provider"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	EnvCommandBase
	Constraints constraints.Value
	UploadTools bool
	Series      []string
	Source      string
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap",
		Purpose: "start up an environment from scratch",
	}
}

func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "set environment constraints")
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
	f.Var(seriesVar{&c.Series}, "series", "upload tools for supplied comma-separated series list")
	f.StringVar(&c.Source, "source", "", "local path to use as tools source")
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
func (c *BootstrapCommand) Run(context *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		if os.IsNotExist(err) {
			out := context.Stderr
			fmt.Fprintln(out, "No juju environment configuration file exists.")
			fmt.Fprintln(out, "Please create a configuration by running:")
			fmt.Fprintln(out, "    juju init -w")
			fmt.Fprintln(out, "then edit the file to configure your juju environment.")
			fmt.Fprintln(out, "You can then re-run bootstrap.")
		}
		return err
	}
	err = c.ensureToolsAvailability(environ, context)
	if err != nil {
		return err
	}
	// TODO: if in verbose mode, write out to Stdout if a new cert was created.
	_, err = environs.EnsureCertificate(environ, environs.WriteCertAndKey)
	if err != nil {
		return err
	}
	// If we are using a local provider, always upload tools.
	if environ.Config().Type() == provider.Local {
		c.UploadTools = true
	}
	if c.UploadTools {
		// Force version.Current, for consistency with subsequent upgrade-juju
		// (see UpgradeJujuCommand).
		forceVersion := uploadVersion(version.Current.Number, nil)
		cfg := environ.Config()
		series := getUploadSeries(cfg, c.Series)
		tools, err := uploadTools(environ.Storage(), &forceVersion, series...)
		if err != nil {
			return err
		}
		cfg, err = cfg.Apply(map[string]interface{}{
			"agent-version": tools.Version.Number.String(),
		})
		if err == nil {
			err = environ.SetConfig(cfg)
		}
		if err != nil {
			return fmt.Errorf("failed to update environment configuration: %v", err)
		}
	}
	return environs.Bootstrap(environ, c.Constraints)
}

// ensureToolsAvailability verifies the tools are available. If no tools are
// found, it will automatically synchronize them.
func (c *BootstrapCommand) ensureToolsAvailability(env environs.Environ, ctx *cmd.Context) error {
	// Capture possible logging while syncing and write it on the screen.
	loggo.RegisterWriter("bootstrap", sync.NewSyncLogWriter(ctx.Stdout, ctx.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("bootstrap")

	// Try to find bootstrap tools.
	_, err := environs.FindBootstrapTools(env, c.Constraints)
	if errors.IsNotFoundError(err) {
		// Not tools available, so synchronize.
		sctx := &sync.SyncContext{
			EnvName: c.EnvName,
			Source:  c.Source,
		}
		if err = syncTools(sctx); err != nil {
			return err
		}
		// Synchronization done, try again.
		_, err = environs.FindBootstrapTools(env, c.Constraints)
	} else if err != nil {
		return err
	}
	return err
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
