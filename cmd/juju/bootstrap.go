// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	cmd.EnvCommandBase
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
func (c *BootstrapCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	environ, err := environs.PrepareFromName(c.EnvName, store)
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
		agenttools, err := uploadTools(environ.Storage(), &forceVersion, series...)
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
	}
	err = c.ensureToolsAvailability(environ, ctx)
	if err != nil {
		return err
	}
	return bootstrap.Bootstrap(environ, c.Constraints)
}

// ensureToolsAvailability verifies the tools are available. If no tools are
// found, it will automatically synchronize them.
func (c *BootstrapCommand) ensureToolsAvailability(env environs.Environ, ctx *cmd.Context) error {
	// Capture possible logging while syncing and write it on the screen.
	loggo.RegisterWriter("bootstrap", cmd.NewCommandLogWriter("juju.environs.sync", ctx.Stdout, ctx.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("bootstrap")

	// Try to find bootstrap tools.
	cfg := env.Config()
	var vers *version.Number
	if agentVersion, ok := cfg.AgentVersion(); ok {
		vers = &agentVersion
	}
	_, err := tools.FindBootstrapTools(
		env, vers, cfg.DefaultSeries(), c.Constraints.Arch, cfg.Development())
	if errors.IsNotFoundError(err) {
		// Not tools available, so synchronize.
		sctx := &sync.SyncContext{
			Target: env.Storage(),
			Source: c.Source,
		}
		if err = syncTools(sctx); err != nil {
			return err
		}
		// Synchronization done, try again.
		_, err = tools.FindBootstrapTools(
			env, vers, cfg.DefaultSeries(), c.Constraints.Arch, cfg.Development())
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
