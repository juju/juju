// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/arch"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var (
	findTools = envtools.FindTools
)

// validateUploadAllowed returns an error if an attempt to upload tools should
// not be allowed.
func validateUploadAllowed(env environs.Environ, toolsArch *string) error {
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
		return errors.Errorf("environment %q of type %s does not support instances running on %q", env.Config().Name(), envType, hostArch)
	}
	return nil
}

// findAvailableTools returns a list of available tools,
// including tools that may be locally built and then
// uploaded. Tools that need to be built will have an
// empty URL.
func findAvailableTools(env environs.Environ, arch *string, upload bool) (coretools.List, error) {
	var availableTools coretools.List
	if upload {
		// We're forcing an upload: ensure we can do so.
		if err := validateUploadAllowed(env, arch); err != nil {
			return nil, err
		}
	} else {
		// We're not forcing an upload, so look for tools
		// in the environment's simplestreams search paths
		// for existing tools.
		var vers *version.Number
		if agentVersion, ok := env.Config().AgentVersion(); ok {
			vers = &agentVersion
		}
		dev := version.Current.IsDev() || env.Config().Development()
		logger.Debugf("looking for bootstrap tools: version=%v", vers)
		toolsList, findToolsErr := findBootstrapTools(env, vers, arch, dev)
		if findToolsErr != nil && !errors.IsNotFound(findToolsErr) {
			return nil, findToolsErr
		}
		// Even if we're successful above, we continue on in case the
		// tools found do not include the local architecture.
		if dev && (vers == nil || version.Current.Number == *vers) {
			// (but only if we're running a dev build,
			// and it's the same as agent-version.)
			if validateUploadAllowed(env, arch) != nil {
				return toolsList, findToolsErr
			}
		} else {
			return toolsList, findToolsErr
		}
		availableTools = toolsList
	}

	// Add tools that we can build locally.
	var archSeries set.Strings
	for _, tools := range availableTools {
		archSeries.Add(tools.Version.Arch + tools.Version.Series)
	}
	for _, series := range version.SupportedSeries() {
		if os, err := version.GetOSFromSeries(series); err != nil || os != version.Ubuntu {
			continue
		}
		if archSeries.Contains(version.Current.Arch + series) {
			continue
		}
		binary := version.Current
		binary.Series = series
		availableTools = append(availableTools, &coretools.Tools{Version: binary})
	}
	return availableTools, nil
}

// findBootstrapTools returns a tools.List containing only those tools with
// which it would be reasonable to launch an environment's first machine,
// given the supplied constraints. If a specific agent version is not requested,
// all tools matching the current major.minor version are chosen.
func findBootstrapTools(env environs.Environ, vers *version.Number, arch *string, dev bool) (list coretools.List, err error) {
	// Construct a tools filter.
	cliVersion := version.Current.Number
	var filter coretools.Filter
	if arch != nil {
		filter.Arch = *arch
	}
	if vers != nil {
		// If we already have an explicit agent version set, we're done.
		filter.Number = *vers
		return findTools(env, cliVersion.Major, cliVersion.Minor, filter, false)
	}
	if !dev {
		logger.Infof("filtering tools by released version")
		filter.Released = true
	}
	return findTools(env, cliVersion.Major, cliVersion.Minor, filter, false)
}
