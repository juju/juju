// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"
	"os"

	"github.com/juju/errors"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/sync"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju/arch"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/version/ubuntu"
)

const noToolsMessage = `Juju cannot bootstrap because no tools are available for your environment.
An attempt was made to build and upload appropriate tools but this was unsuccessful.
`

const noToolsNoUploadMessage = `Juju cannot bootstrap because no tools are available for your environment.
You may want to use the 'tools-metadata-url' configuration setting to specify the tools location.
`

// UploadTools uploads tools for the specified series and any other relevant series to
// the environment storage, after which it sets the agent-version. If forceVersion is true,
// we allow uploading even when the agent-version is already set in the environment.
func UploadTools(ctx environs.BootstrapContext, env environs.Environ, toolsArch *string, forceVersion bool, bootstrapSeries ...string) error {
	logger.Infof("checking that upload is possible")
	// Check the series are valid.
	for _, series := range bootstrapSeries {
		if _, err := ubuntu.SeriesVersion(series); err != nil {
			return err
		}
	}
	// See that we are allowed to upload the tools.
	if err := validateUploadAllowed(env, toolsArch, forceVersion); err != nil {
		return err
	}

	// Make storage interruptible.
	interrupted := make(chan os.Signal, 1)
	interruptStorage := make(chan struct{})
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)
	defer close(interrupted)
	go func() {
		defer close(interruptStorage) // closing interrupts all uploads
		if _, ok := <-interrupted; ok {
			ctx.Infof("cancelling tools upload")
		}
	}()
	stor := newInterruptibleStorage(env.Storage(), interruptStorage)

	cfg := env.Config()
	explicitVersion := uploadVersion(version.Current.Number, nil)
	uploadSeries := SeriesToUpload(cfg, bootstrapSeries)
	ctx.Infof("uploading tools for series %s", uploadSeries)
	tools, err := sync.Upload(stor, &explicitVersion, uploadSeries...)
	if err != nil {
		return err
	}
	cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": tools.Version.Number.String(),
	})
	if err == nil {
		err = env.SetConfig(cfg)
	}
	if err != nil {
		return fmt.Errorf("failed to update environment configuration: %v", err)
	}
	return nil
}

// uploadVersion returns a copy of the supplied version with a build number
// higher than any of the supplied tools that share its major, minor and patch.
func uploadVersion(vers version.Number, existing coretools.List) version.Number {
	vers.Build++
	for _, t := range existing {
		if t.Version.Major != vers.Major || t.Version.Minor != vers.Minor || t.Version.Patch != vers.Patch {
			continue
		}
		if t.Version.Build >= vers.Build {
			vers.Build = t.Version.Build + 1
		}
	}
	return vers
}

// Unless otherwise specified, we will upload tools for all lts series on bootstrap
// when --upload-tools is used.
// ToolsLtsSeries records the known lts series.
var ToolsLtsSeries = []string{"precise", "trusty"}

// SeriesToUpload returns the supplied series with duplicates removed if
// non-empty; otherwise it returns a default list of series we should
// probably upload, based on cfg.
func SeriesToUpload(cfg *config.Config, series []string) []string {
	unique := set.NewStrings(series...)
	if unique.IsEmpty() {
		unique.Add(version.Current.Series)
		for _, toolsSeries := range ToolsLtsSeries {
			unique.Add(toolsSeries)
		}
		if series, ok := cfg.DefaultSeries(); ok {
			unique.Add(series)
		}
	}
	return unique.Values()
}

// validateUploadAllowed returns an error if an attempt to upload tools should
// not be allowed.
func validateUploadAllowed(env environs.Environ, toolsArch *string, forceVersion bool) error {
	if !forceVersion {
		// First, check that there isn't already an agent version specified.
		if _, hasAgentVersion := env.Config().AgentVersion(); hasAgentVersion {
			return fmt.Errorf(noToolsNoUploadMessage)
		}
	}
	// Now check that the architecture for which we are setting up an
	// environment matches that from which we are bootstrapping.
	hostArch := arch.HostArch()
	// We can't build tools for a different architecture if one is specified.
	if toolsArch != nil && *toolsArch != hostArch {
		return fmt.Errorf("cannot build tools for %q using a machine running on %q", *toolsArch, hostArch)
	}
	// If no architecture is specified, ensure the target provider supports instances matching our architecture.
	supportedArchitectures, err := env.SupportedArchitectures()
	if err != nil {
		return fmt.Errorf(
			"no packaged tools available and cannot determine environment's supported architectures: %v", err)
	}
	archSupported := false
	for _, arch := range supportedArchitectures {
		if hostArch == arch {
			archSupported = true
			break
		}
	}
	if !archSupported {
		envType := env.Config().Type()
		return fmt.Errorf(
			"environment %q of type %s does not support instances running on %q", env.Name(), envType, hostArch)
	}
	return nil
}

// EnsureToolsAvailability verifies the tools are available. If no tools are
// found, it will automatically synchronize them.
func EnsureToolsAvailability(ctx environs.BootstrapContext, env environs.Environ, series string, toolsArch *string) (coretools.List, error) {
	cfg := env.Config()
	var vers *version.Number
	if agentVersion, ok := cfg.AgentVersion(); ok {
		vers = &agentVersion
	}

	logger.Debugf(
		"looking for bootstrap tools: series=%q, arch=%v, version=%v",
		series, toolsArch, vers,
	)
	params := envtools.BootstrapToolsParams{
		Version: vers,
		Arch:    toolsArch,
		Series:  series,
		// If vers.Build>0, the tools may have been uploaded in this session.
		// Allow retries, so we wait until the storage has caught up.
		AllowRetry: vers != nil && vers.Build > 0,
	}
	toolsList, err := envtools.FindBootstrapTools(env, params)
	if err == nil {
		return toolsList, nil
	} else if !errors.IsNotFound(err) {
		return nil, err
	}

	// Only automatically upload tools for dev versions.
	if !version.Current.IsDev() {
		return nil, fmt.Errorf("cannot upload bootstrap tools: %v", noToolsNoUploadMessage)
	}

	// No tools available so our only hope is to build locally and upload.
	logger.Warningf("no prepackaged tools available")
	uploadSeries := SeriesToUpload(cfg, nil)
	if series != "" {
		uploadSeries = append(uploadSeries, series)
	}
	if err := UploadTools(ctx, env, toolsArch, false, uploadSeries...); err != nil {
		logger.Errorf("%s", noToolsMessage)
		return nil, fmt.Errorf("cannot upload bootstrap tools: %v", err)
	}
	// TODO(axw) have uploadTools return the list of tools in the target, and use that.
	params.AllowRetry = true
	if toolsList, err = envtools.FindBootstrapTools(env, params); err != nil {
		return nil, fmt.Errorf("cannot find bootstrap tools: %v", err)
	}
	return toolsList, nil
}
