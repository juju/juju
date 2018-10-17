// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"

	"github.com/juju/errors"
	jujuos "github.com/juju/os"
	"github.com/juju/os/series"
	"github.com/juju/utils/arch"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var (
	findTools = envtools.FindTools
)

// validateUploadAllowed returns an error if an attempt to upload tools should
// not be allowed.
func validateUploadAllowed(env environs.ConfigGetter, toolsArch, toolsSeries *string, validator constraints.Validator) error {
	// Now check that the architecture and series for which we are setting up an
	// environment matches that from which we are bootstrapping.
	hostArch := arch.HostArch()
	// We can't build tools for a different architecture if one is specified.
	if toolsArch != nil && *toolsArch != hostArch {
		return fmt.Errorf("cannot use agent built for %q using a machine running on %q", *toolsArch, hostArch)
	}
	hostOS := jujuos.HostOS()
	if toolsSeries != nil {
		toolsSeriesOS, err := series.GetOSFromSeries(*toolsSeries)
		if err != nil {
			return errors.Trace(err)
		}
		if !toolsSeriesOS.EquivalentTo(hostOS) {
			return errors.Errorf("cannot use agent built for %q using a machine running %q", *toolsSeries, hostOS)
		}
	}
	// If no architecture is specified, ensure the target provider supports instances matching our architecture.
	if _, err := validator.Validate(constraints.Value{Arch: &hostArch}); err != nil {
		return errors.Errorf(
			"model %q of type %s does not support instances running on %q",
			env.Config().Name(), env.Config().Type(), hostArch,
		)
	}
	return nil
}

// findPackagedTools returns a list of tools for in simplestreams.
func findPackagedTools(
	env environs.BootstrapEnviron,
	vers *version.Number,
	arch, series *string,
) (coretools.List, error) {
	// Look for tools in the environment's simplestreams search paths
	// for existing tools.

	// If the user hasn't asked for a specified tools version, see if
	// one is configured in the environment.
	if vers == nil {
		if agentVersion, ok := env.Config().AgentVersion(); ok {
			vers = &agentVersion
		}
	}
	logger.Infof("looking for bootstrap agent binaries: version=%v", vers)
	toolsList, findToolsErr := findBootstrapTools(env, vers, arch, series)
	logger.Infof("found %d packaged agent binaries", len(toolsList))
	if findToolsErr != nil {
		return nil, findToolsErr
	}
	return toolsList, nil
}

// locallyBuildableTools returns the list of tools that
// can be built locally, for series of the same OS.
func locallyBuildableTools(toolsSeries *string) (buildable coretools.List, _ version.Number) {
	buildNumber := jujuversion.Current
	// Increment the build number so we know it's a custom build.
	buildNumber.Build++
	for _, ser := range series.SupportedSeries() {
		if os, err := series.GetOSFromSeries(ser); err != nil || !os.EquivalentTo(jujuos.HostOS()) {
			continue
		}
		if toolsSeries != nil && ser != *toolsSeries {
			continue
		}
		binary := version.Binary{
			Number: buildNumber,
			Series: ser,
			Arch:   arch.HostArch(),
		}
		buildable = append(buildable, &coretools.Tools{Version: binary})
	}
	return buildable, buildNumber
}

// findBootstrapTools returns a tools.List containing only those tools with
// which it would be reasonable to launch an environment's first machine,
// given the supplied constraints. If a specific agent version is not requested,
// all tools matching the current major.minor version are chosen.
func findBootstrapTools(env environs.BootstrapEnviron, vers *version.Number, arch, series *string) (list coretools.List, err error) {
	// Construct a tools filter.
	cliVersion := jujuversion.Current
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
	streams := envtools.PreferredStreams(vers, env.Config().Development(), env.Config().AgentStream())
	return findTools(env, cliVersion.Major, cliVersion.Minor, streams, filter)
}
