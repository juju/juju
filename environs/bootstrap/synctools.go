// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/sync"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/arch"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

const noToolsMessage = `Juju cannot bootstrap because no tools are available for your environment.
An attempt was made to build and upload appropriate tools but this was unsuccessful.
`

const noToolsNoUploadMessage = `Juju cannot bootstrap because no tools are available for your environment.
You may want to use the 'tools-metadata-url' configuration setting to specify the tools location.
`

func uploadTools(env environs.Environ, bootstrapSeries string) error {
	cfg := env.Config()
	uploadVersion := version.Current.Number
	uploadVersion.Build++
	uploadSeries := set.NewStrings(
		bootstrapSeries,
		cfg.DefaultSeries(),
		config.DefaultSeries,
	)
	tools, err := sync.Upload(env.Storage(), &uploadVersion, uploadSeries.Values()...)
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

// EnsureToolsAvailability verifies the tools are available. If no tools are
// found, it will automatically synchronize them.
func EnsureToolsAvailability(env environs.Environ, series string, toolsArch *string) (coretools.List, error) {
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
	} else if !errors.IsNotFoundError(err) {
		return nil, err
	}

	// No tools available so our only hope is to build locally and upload.
	// First, check that there isn't already an agent version specified, and that we
	// are running a development version.
	if _, hasAgentVersion := env.Config().AgentVersion(); hasAgentVersion || !version.Current.IsDev() {
		return nil, fmt.Errorf(noToolsNoUploadMessage)
	}

	// Now check that the architecture for which we are setting up an
	// environment matches that from which we are bootstrapping.
	hostArch, err := arch.HostArch()
	if err != nil {
		return nil, fmt.Errorf(
			"no packaged tools available and cannot determine local architecure to build custom tools: %v", err)
	}
	// We can't build tools for a different architecture if one is specified.
	if toolsArch != nil && *toolsArch != hostArch {
		return nil, fmt.Errorf("cannot build tools for %q using a machine running on %q", *toolsArch, hostArch)
	}
	// If no architecture is specified, ensure the target provider supports instances matching our architecture.
	supportedArchitectures, err := env.SupportedArchitectures()
	if err != nil {
		return nil, fmt.Errorf(
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
		return nil, fmt.Errorf(
			"environment %q of type %s does not support instances running on %q", env.Name(), cfg.Type(), hostArch)
	}
	// So we now know we can build/package tools for an architecture which can be hosted by the environment.
	logger.Warningf("no tools available, attempting to upload")
	if err := uploadTools(env, series); err != nil {
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
