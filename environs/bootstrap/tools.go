// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var (
	findTools = envtools.FindTools
)

// validateUploadAllowed returns an error if an attempt to upload tools should
// not be allowed.
func validateUploadAllowed(env environs.Environ, toolsArch, toolsSeries *string) error {
	// Now check that the architecture and series for which we are setting up an
	// environment matches that from which we are bootstrapping.
	hostArch := arch.HostArch()
	// We can't build tools for a different architecture if one is specified.
	if toolsArch != nil && *toolsArch != hostArch {
		return fmt.Errorf("cannot build tools for %q using a machine running on %q", *toolsArch, hostArch)
	}
	hostOS := jujuos.HostOS()
	if toolsSeries != nil {
		toolsSeriesOS, err := series.GetOSFromSeries(*toolsSeries)
		if err != nil {
			return errors.Trace(err)
		}
		if toolsSeriesOS != hostOS {
			return errors.Errorf("cannot build tools for %q using a machine running %q", *toolsSeries, hostOS)
		}
	}
	// If no architecture is specified, ensure the target provider supports instances matching our architecture.
	supportedArchitectures, err := env.SupportedArchitectures()
	if err != nil {
		return fmt.Errorf(
			"no packaged tools available and cannot determine model's supported architectures: %v", err)
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
		return errors.Errorf("model %q of type %s does not support instances running on %q", env.Config().Name(), envType, hostArch)
	}
	return nil
}

// findAvailableTools returns a list of available tools,
// including tools that may be locally built and then
// uploaded. Tools that need to be built will have an
// empty URL.
func findAvailableTools(env environs.Environ, vers *version.Number, arch, series *string, upload bool) (coretools.List, error) {
	if upload {
		// We're forcing an upload: ensure we can do so.
		if err := validateUploadAllowed(env, arch, series); err != nil {
			return nil, err
		}
		return locallyBuildableTools(series), nil
	}

	// We're not forcing an upload, so look for tools
	// in the environment's simplestreams search paths
	// for existing tools.

	// If the user hasn't asked for a specified tools version, see if
	// one is configured in the environment.
	if vers == nil {
		if agentVersion, ok := env.Config().AgentVersion(); ok {
			vers = &agentVersion
		}
	}
	logger.Infof("looking for bootstrap tools: version=%v", vers)
	toolsList, findToolsErr := findBootstrapTools(env, vers, arch, series)
	if findToolsErr != nil && !errors.IsNotFound(findToolsErr) {
		return nil, findToolsErr
	}

	preferredStream := envtools.PreferredStream(vers, env.Config().Development(), env.Config().AgentStream())
	if preferredStream == envtools.ReleasedStream || vers != nil {
		// We are not running a development build, or agent-version
		// was specified; the only tools available are the ones we've
		// just found.
		return toolsList, findToolsErr
	}
	// The tools located may not include the ones that the
	// provider requires. We are running a development build,
	// so augment the list of tools with those that we can build
	// locally.

	// Collate the set of arch+series that are externally available
	// so we can see if we need to build any locally. If we need
	// to, only then do we validate that we can upload (which
	// involves a potentially expensive SupportedArchitectures call).
	archSeries := make(set.Strings)
	for _, tools := range toolsList {
		archSeries.Add(tools.Version.Arch + tools.Version.Series)
	}
	var localToolsList coretools.List
	for _, tools := range locallyBuildableTools(series) {
		if !archSeries.Contains(tools.Version.Arch + tools.Version.Series) {
			localToolsList = append(localToolsList, tools)
		}
	}
	if len(localToolsList) == 0 || validateUploadAllowed(env, arch, series) != nil {
		return toolsList, findToolsErr
	}
	return append(toolsList, localToolsList...), nil
}

// locallyBuildableTools returns the list of tools that
// can be built locally, for series of the same OS.
func locallyBuildableTools(toolsSeries *string) (buildable coretools.List) {
	for _, ser := range series.SupportedSeries() {
		if os, err := series.GetOSFromSeries(ser); err != nil || os != jujuos.HostOS() {
			continue
		}
		if toolsSeries != nil && ser != *toolsSeries {
			continue
		}
		binary := version.Binary{
			Number: version.Current,
			Series: ser,
			Arch:   arch.HostArch(),
		}
		// Increment the build number so we know it's a development build.
		binary.Build++
		buildable = append(buildable, &coretools.Tools{Version: binary})
	}
	return buildable
}

// findBootstrapTools returns a tools.List containing only those tools with
// which it would be reasonable to launch an environment's first machine,
// given the supplied constraints. If a specific agent version is not requested,
// all tools matching the current major.minor version are chosen.
func findBootstrapTools(env environs.Environ, vers *version.Number, arch, series *string) (list coretools.List, err error) {
	// Construct a tools filter.
	cliVersion := version.Current
	var filter coretools.Filter
	if arch != nil {
		filter.Arch = *arch
	}
	if series != nil {
		filter.Series = *series
	}
	if vers != nil {
		filter.Number = *vers
	}
	stream := envtools.PreferredStream(vers, env.Config().Development(), env.Config().AgentStream())
	return findTools(env, cliVersion.Major, cliVersion.Minor, stream, filter)
}
