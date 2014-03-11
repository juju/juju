// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/environs/tools"
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

Bootstrap initializes the cloud environment synchronously and displays information
about the current installation steps.  The time for bootstrap to complete varies 
across cloud providers from a few seconds to several minutes.  Once bootstrap has 
completed, you can run other juju commands against your environment. You can change
the default timeout and retry delays used during the bootstrap by changing the
following settings in your environments.yaml (all values represent number of seconds):

    # How long to wait for a connection to the state server.
    bootstrap-timeout: 600 # default: 10 minutes
    # How long to wait between connection attempts to a state server address.
    bootstrap-retry-delay: 5 # default: 5 seconds
    # How often to refresh state server addresses from the API server.
    bootstrap-addresses-delay: 10 # default: 10 seconds

Private clouds may need to specify their own custom image metadata, and possibly upload
Juju tools to cloud storage if no outgoing Internet access is available. In this case,
use the --metadata-source paramater to tell bootstrap a local directory from which to
upload tools and/or image metadata.

See Also:
   juju help switch
   juju help constraints
   juju help set-constraints
`

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	cmd.EnvCommandBase
	Constraints    constraints.Value
	UploadTools    bool
	Series         []string
	MetadataSource string
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
	f.StringVar(&c.MetadataSource, "metadata-source", "", "local path to use as tools and/or metadata source")
}

func (c *BootstrapCommand) Init(args []string) (err error) {
	if len(c.Series) > 0 && !c.UploadTools {
		return fmt.Errorf("--series requires --upload-tools")
	}
	return cmd.CheckEmpty(args)
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *BootstrapCommand) Run(ctx *cmd.Context) (resultErr error) {
	environ, cleanup, err := environFromName(ctx, c.EnvName, &resultErr, "Bootstrap")
	if err != nil {
		return err
	}
	defer cleanup()
	if err := bootstrap.EnsureNotBootstrapped(environ); err != nil {
		return err
	}
	// If --metadata-source is specified, override the default tools metadata source so
	// SyncTools can use it, and also upload any image metadata.
	if c.MetadataSource != "" {
		metadataDir := ctx.AbsPath(c.MetadataSource)
		logger.Infof("Setting default tools and image metadata sources: %s", metadataDir)
		tools.DefaultBaseURL = metadataDir
		if err := imagemetadata.UploadImageMetadata(environ.Storage(), metadataDir); err != nil {
			// Do not error if image metadata directory doesn't exist.
			if !os.IsNotExist(err) {
				return fmt.Errorf("uploading image metadata: %v", err)
			}
		} else {
			logger.Infof("custom image metadata uploaded")
		}
	}
	// TODO (wallyworld): 2013-09-20 bug 1227931
	// We can set a custom tools data source instead of doing an
	// unnecessary upload.
	if environ.Config().Type() == provider.Local {
		c.UploadTools = true
	}
	if c.UploadTools {
		err = c.uploadTools(environ)
		if err != nil {
			return err
		}
	}
	return bootstrap.Bootstrap(ctx, environ, c.Constraints)
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
