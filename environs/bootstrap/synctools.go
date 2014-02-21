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
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

const noToolsMessage = `Juju cannot bootstrap because no tools are available for your environment.
An attempt was made to build and upload appropriate tools but this was unsuccessful.
`

const noToolsNoUploadMessage = `Juju cannot bootstrap because no tools are available for your environment.
In addition, no tools could be located to upload.
You may want to use the 'tools-metadata-url' configuration setting to specify the tools location.
`

// syncOrUpload first attempts to synchronize tools from
// the default tools source to the environment's storage.
//
// If synchronization fails due to no matching tools,
// a development version of juju is running, and no
// agent-version has been specified, then attempt to
// build and upload local tools.
func syncOrUpload(env environs.Environ, bootstrapSeries string) error {
	sctx := &sync.SyncContext{
		Target: env.Storage(),
	}
	err := sync.SyncTools(sctx)
	if err == coretools.ErrNoMatches || err == envtools.ErrNoTools {
		if _, hasAgentVersion := env.Config().AgentVersion(); !hasAgentVersion && version.Current.IsDev() {
			logger.Warningf("no tools found, so attempting to build and upload new tools")
			if err = uploadTools(env, bootstrapSeries); err != nil {
				logger.Errorf("%s", noToolsMessage)
				return err
			}
		} else {
			logger.Errorf("%s", noToolsNoUploadMessage)
		}
	}
	return err
}

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
func EnsureToolsAvailability(env environs.Environ, series string, arch *string) (coretools.List, error) {
	cfg := env.Config()
	var vers *version.Number
	if agentVersion, ok := cfg.AgentVersion(); ok {
		vers = &agentVersion
	}

	logger.Debugf(
		"looking for bootstrap tools: series=%q, arch=%v, version=%v",
		series, arch, vers,
	)
	params := envtools.BootstrapToolsParams{
		Version: vers,
		Arch:    arch,
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

	// No tools available, so synchronize.
	logger.Warningf("no tools available, attempting to retrieve from %v", envtools.DefaultBaseURL)
	if syncErr := syncOrUpload(env, series); syncErr != nil {
		// The target may have tools that don't match, so don't
		// return a misleading "no tools found" error.
		if syncErr != envtools.ErrNoTools {
			err = syncErr
		}
		return nil, fmt.Errorf("cannot find bootstrap tools: %v", err)
	}
	// TODO(axw) have syncOrUpload return the list of tools in the target, and use that.
	params.AllowRetry = true
	if toolsList, err = envtools.FindBootstrapTools(env, params); err != nil {
		return nil, fmt.Errorf("cannot find bootstrap tools: %v", err)
	}
	return toolsList, nil
}
