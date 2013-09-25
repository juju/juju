// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
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
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider"
	coretools "launchpad.net/juju-core/tools"
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
	// If the environment has a special bootstrap Storage, use it wherever
	// we'd otherwise use environ.Storage.
	if bs, ok := environ.(environs.BootstrapStorager); ok {
		bootstrapStorage, err := bs.BootstrapStorage()
		if err != nil {
			return fmt.Errorf("failed to acquire bootstrap storage: %v", err)
		}
		environ = &bootstrapStorageEnviron{environ, bootstrapStorage}
	}
	// TODO: if in verbose mode, write out to Stdout if a new cert was created.
	_, err = environs.EnsureCertificate(environ, environs.WriteCertAndKey)
	if err != nil {
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
	err = c.ensureToolsAvailability(environ, ctx, c.UploadTools)
	if err != nil {
		return err
	}
	return bootstrap.Bootstrap(environ, c.Constraints)
}

func (c *BootstrapCommand) uploadTools(environ environs.Environ) error {
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
	return nil
}

const NoToolsMessage = `
Juju cannot bootstrap because no tools are available for your environment.
An attempt was made to build and upload appropriate tools but this was unsuccessful.

`

const NoToolsNoUploadMessage = `
Juju cannot bootstrap because no tools are available for your environment.
In addition, no tools could be located to upload.
You may want to use the 'tools-url' configuration setting to specify the tools location.

`

func processToolsError(w io.Writer, err *error, uploadAttempted *bool) {

	if *uploadAttempted && *err != nil {
		fmt.Fprint(w, NoToolsMessage)
	} else {
		if errors.IsNotFoundError(*err) || *err == coretools.ErrNoMatches {
			fmt.Fprint(w, NoToolsNoUploadMessage)
		}
	}
}

// ensureToolsAvailability verifies the tools are available. If no tools are
// found, it will automatically synchronize them.
func (c *BootstrapCommand) ensureToolsAvailability(env environs.Environ, ctx *cmd.Context, uploadPerformed bool) (err error) {
	uploadAttempted := false
	defer processToolsError(ctx.Stderr, &err, &uploadAttempted)
	// Capture possible logging while syncing and write it on the screen.
	loggo.RegisterWriter("bootstrap", cmd.NewCommandLogWriter("juju.environs.sync", ctx.Stdout, ctx.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("bootstrap")

	// Try to find bootstrap tools.
	cfg := env.Config()
	var vers *version.Number
	if agentVersion, ok := cfg.AgentVersion(); ok {
		vers = &agentVersion
	}
	logger.Debugf("looking for bootstrap tools")
	params := envtools.BootstrapToolsParams{
		Version:    vers,
		Arch:       c.Constraints.Arch,
		AllowRetry: uploadPerformed,
	}
	_, err = envtools.FindBootstrapTools(env, params)
	if errors.IsNotFoundError(err) {
		// No tools available, so synchronize.
		toolsSource := c.Source
		if c.Source == "" {
			toolsSource = sync.DefaultToolsLocation
		}
		logger.Warningf("no tools available, attempting to retrieve from %v", toolsSource)
		sctx := &sync.SyncContext{
			Target: env.Storage(),
			Source: c.Source,
		}
		if err = syncTools(sctx); err != nil {
			if err == coretools.ErrNoMatches && vers == nil && version.Current.IsDev() {
				logger.Infof("no tools found, so attempting to build and upload new tools")
				uploadAttempted = true
				if err = c.uploadTools(env); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		// Synchronization done, try again.
		params.AllowRetry = true
		_, err = envtools.FindBootstrapTools(env, params)
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

type bootstrapStorageEnviron struct {
	environs.Environ
	bootstrapStorage storage.Storage
}

func (b *bootstrapStorageEnviron) Storage() storage.Storage {
	return b.bootstrapStorage
}
