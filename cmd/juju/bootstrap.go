// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider"
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
	envcmd.EnvCommandBase
	Constraints    constraints.Value
	UploadTools    bool
	Series         []string
	seriesOld      []string
	MetadataSource string
	Placement      string
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap",
		Purpose: "start up an environment from scratch",
		Doc:     bootstrapDoc,
	}
}

func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "set environment constraints")
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
	f.Var(newSeriesValue(nil, &c.Series), "upload-series", "upload tools for supplied comma-separated series list")
	f.Var(newSeriesValue(nil, &c.seriesOld), "series", "upload tools for supplied comma-separated series list (DEPRECATED, see --upload-series)")
	f.StringVar(&c.MetadataSource, "metadata-source", "", "local path to use as tools and/or metadata source")
	f.StringVar(&c.Placement, "to", "", "a placement directive indicating an instance to bootstrap")
}

func (c *BootstrapCommand) Init(args []string) (err error) {
	if len(c.Series) > 0 && !c.UploadTools {
		return fmt.Errorf("--upload-series requires --upload-tools")
	}
	if len(c.seriesOld) > 0 && !c.UploadTools {
		return fmt.Errorf("--series requires --upload-tools")
	}
	if len(c.Series) > 0 && len(c.seriesOld) > 0 {
		return fmt.Errorf("--upload-series and --series can't be used together")
	}
	if len(c.seriesOld) > 0 {
		c.Series = c.seriesOld
	}

	// Parse the placement directive. Bootstrap currently only
	// supports provider-specific placement directives.
	if c.Placement != "" {
		_, err = instance.ParsePlacement(c.Placement)
		if err != instance.ErrPlacementScopeMissing {
			// We only support unscoped placement directives for bootstrap.
			return fmt.Errorf("unsupported bootstrap placement directive %q", c.Placement)
		}
	}
	return cmd.CheckEmpty(args)
}

type seriesValue struct {
	*cmd.StringsValue
}

// newSeriesValue is used to create the type passed into the gnuflag.FlagSet Var function.
func newSeriesValue(defaultValue []string, target *[]string) *seriesValue {
	v := seriesValue{(*cmd.StringsValue)(target)}
	*(v.StringsValue) = defaultValue
	return &v
}

// Implements gnuflag.Value Set.
func (v *seriesValue) Set(s string) error {
	if err := v.StringsValue.Set(s); err != nil {
		return err
	}
	for _, name := range *(v.StringsValue) {
		if !charm.IsValidSeries(name) {
			v.StringsValue = nil
			return fmt.Errorf("invalid series name %q", name)
		}
	}
	return nil
}

// bootstrap functionality that Run calls to support cleaner testing
type BootstrapInterface interface {
	EnsureNotBootstrapped(env environs.Environ) error
	UploadTools(environs.BootstrapContext, environs.Environ, *string, bool, ...string) error
	Bootstrap(ctx environs.BootstrapContext, environ environs.Environ, args environs.BootstrapParams) error
}

type bootstrapFuncs struct{}

func (b bootstrapFuncs) EnsureNotBootstrapped(env environs.Environ) error {
	return bootstrap.EnsureNotBootstrapped(env)
}

func (b bootstrapFuncs) UploadTools(ctx environs.BootstrapContext, env environs.Environ, toolsArch *string, forceVersion bool, bootstrapSeries ...string) error {
	return bootstrap.UploadTools(ctx, env, toolsArch, forceVersion, bootstrapSeries...)
}

func (b bootstrapFuncs) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams) error {
	return bootstrap.Bootstrap(ctx, env, args)
}

var getBootstrapFuncs = func() BootstrapInterface {
	return &bootstrapFuncs{}
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *BootstrapCommand) Run(ctx *cmd.Context) (resultErr error) {
	bootstrapFuncs := getBootstrapFuncs()

	if len(c.seriesOld) > 0 {
		fmt.Fprintln(ctx.Stderr, "Use of --series is deprecated. Please use --upload-series instead.")
	}

	environ, cleanup, err := environFromName(ctx, c.EnvName, &resultErr, "Bootstrap")
	if err != nil {
		return err
	}
	// We want to validate constraints early. However, if a custom image metadata
	// source is specified, we can't validate the arch because that depends on what
	// images metadata is to be uploaded. So we validate here if no custom metadata
	// source is specified, and defer till later if not.
	if c.MetadataSource == "" {
		if err := validateConstraints(c.Constraints, environ); err != nil {
			return err
		}
	}

	defer cleanup()
	if err := bootstrapFuncs.EnsureNotBootstrapped(environ); err != nil {
		return err
	}

	// Block interruption during bootstrap. Providers may also
	// register for interrupt notification so they can exit early.
	interrupted := make(chan os.Signal, 1)
	defer close(interrupted)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)
	go func() {
		for _ = range interrupted {
			ctx.Infof("Interrupt signalled: waiting for bootstrap to exit")
		}
	}()

	// If --metadata-source is specified, override the default tools metadata source so
	// SyncTools can use it, and also upload any image metadata.
	if c.MetadataSource != "" {
		metadataDir := ctx.AbsPath(c.MetadataSource)
		if err := uploadCustomMetadata(metadataDir, environ); err != nil {
			return err
		}
		if err := validateConstraints(c.Constraints, environ); err != nil {
			return err
		}
	}
	// TODO (wallyworld): 2013-09-20 bug 1227931
	// We can set a custom tools data source instead of doing an
	// unnecessary upload.
	if environ.Config().Type() == provider.Local {
		c.UploadTools = true
	}
	if c.UploadTools {
		err = bootstrapFuncs.UploadTools(ctx, environ, c.Constraints.Arch, true, c.Series...)
		if err != nil {
			return err
		}
	}
	return bootstrapFuncs.Bootstrap(ctx, environ, environs.BootstrapParams{
		Constraints: c.Constraints,
		Placement:   c.Placement,
	})
}

var uploadCustomMetadata = func(metadataDir string, env environs.Environ) error {
	logger.Infof("Setting default tools and image metadata sources: %s", metadataDir)
	tools.DefaultBaseURL = metadataDir
	if err := imagemetadata.UploadImageMetadata(env.Storage(), metadataDir); err != nil {
		// Do not error if image metadata directory doesn't exist.
		if !os.IsNotExist(err) {
			return fmt.Errorf("uploading image metadata: %v", err)
		}
	} else {
		logger.Infof("custom image metadata uploaded")
	}
	return nil
}

var validateConstraints = func(cons constraints.Value, env environs.Environ) error {
	validator, err := env.ConstraintsValidator()
	if err != nil {
		return err
	}
	unsupported, err := validator.Validate(cons)
	if len(unsupported) > 0 {
		logger.Warningf("unsupported constraints: %v", err)
	} else if err != nil {
		return err
	}
	return nil
}
