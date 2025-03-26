// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	internallogger "github.com/juju/juju/internal/logger"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
)

var logger = internallogger.GetLogger("juju.environs.tools")

func makeToolsConstraint(ctx context.Context,
	cloudSpec simplestreams.CloudSpec, stream string, majorVersion, minorVersion int,
	filter coretools.Filter,
) (*ToolsConstraint, error) {

	var toolsConstraint *ToolsConstraint
	if filter.Number != version.Zero {
		// A specific tools version is required, however, a general match based on major/minor
		// version may also have been requested. This is used to ensure any agent version currently
		// recorded in the environment matches the Juju cli version.
		// We can short circuit any lookup here by checking the major/minor numbers against
		// the filter version and exiting early if there is a mismatch.
		majorMismatch := majorVersion > 0 && majorVersion != filter.Number.Major
		minorMismacth := minorVersion != -1 && minorVersion != filter.Number.Minor
		if majorMismatch || minorMismacth {
			return nil, coretools.ErrNoMatches
		}
		toolsConstraint = NewVersionedToolsConstraint(filter.Number,
			simplestreams.LookupParams{CloudSpec: cloudSpec, Stream: stream})
	} else {
		toolsConstraint = NewGeneralToolsConstraint(majorVersion, minorVersion,
			simplestreams.LookupParams{CloudSpec: cloudSpec, Stream: stream})
	}
	if filter.Arch != "" {
		toolsConstraint.Arches = []string{filter.Arch}
	} else {
		logger.Tracef(ctx, "no architecture specified when finding agent binaries, looking for any")
		toolsConstraint.Arches = arch.AllSupportedArches
	}
	var osToSearch []string
	if filter.OSType != "" {
		osToSearch = []string{filter.OSType}
	} else {
		osToSearch = []string{corebase.UbuntuOS}
		logger.Tracef(ctx, "no os type specified when finding agent binaries, looking for ubuntu")
	}
	toolsConstraint.Releases = osToSearch
	return toolsConstraint, nil
}

// FindTools returns a List containing all tools in the given stream, with a given
// major.minor version number available in the cloud instance, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindTools(ctx context.Context, ss SimplestreamsFetcher, env environs.BootstrapEnviron,
	majorVersion, minorVersion int, streams []string, filter coretools.Filter,
) (_ coretools.List, err error) {
	var cloudSpec simplestreams.CloudSpec

	switch env := env.(type) {
	case simplestreams.HasRegion:
		if cloudSpec, err = env.Region(); err != nil {
			return nil, err
		}
	}
	// If only one of region or endpoint is provided, that is a problem.
	if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
		return nil, errors.New("cannot find agent binaries without a complete cloud configuration")
	}

	logger.Debugf(ctx, "finding agent binaries in stream: %q", strings.Join(streams, ", "))
	if minorVersion >= 0 {
		logger.Debugf(ctx, "reading agent binaries with major.minor version %d.%d", majorVersion, minorVersion)
	} else if majorVersion > 0 {
		logger.Debugf(ctx, "reading agent binaries with major version %d", majorVersion)
	}
	defer convertToolsError(&err)

	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	if filter.Number != version.Zero {
		logger.Debugf(ctx, "filtering agent binaries by version: %s", filter.Number)
	}
	if filter.OSType != "" {
		logger.Debugf(ctx, "filtering agent binaries by os type: %s", filter.OSType)
	}
	if filter.Arch != "" {
		logger.Debugf(ctx, "filtering agent binaries by architecture: %s", filter.Arch)
	}

	sources, err := GetMetadataSources(env, ss)
	if err != nil {
		return nil, err
	}
	return FindToolsForCloud(ctx, ss, sources, cloudSpec, streams, majorVersion, minorVersion, filter)
}

// FindToolsForCloud returns a List containing all tools in the given streams, with a given
// major.minor version number and cloudSpec, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindToolsForCloud(ctx context.Context, ss SimplestreamsFetcher,
	sources []simplestreams.DataSource, cloudSpec simplestreams.CloudSpec, streams []string,
	majorVersion, minorVersion int, filter coretools.Filter) (coretools.List, error) {
	var (
		list         coretools.List
		noToolsCount int

		seenBinary = make(map[version.Binary]bool)
	)
	for _, stream := range streams {
		toolsConstraint, err := makeToolsConstraint(ctx, cloudSpec, stream, majorVersion, minorVersion, filter)
		if err != nil {
			return nil, err
		}
		toolsMetadata, _, err := Fetch(ctx, ss, sources, toolsConstraint)
		if errors.Is(err, errors.NotFound) {
			noToolsCount++
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, metadata := range toolsMetadata {
			binary, err := metadata.binary()
			if err != nil {
				return nil, errors.Trace(err)
			}
			// Ensure that we only add an agent version if we haven't
			// already seen it from a more preferred stream.
			if seenBinary[binary] {
				continue
			}
			list = append(list, &coretools.Tools{
				Version: binary,
				URL:     metadata.FullPath,
				Size:    metadata.Size,
				SHA256:  metadata.SHA256,
			})
			seenBinary[binary] = true
		}
	}
	if len(list) == 0 {
		if len(streams) == noToolsCount {
			return nil, ErrNoTools
		}
		return nil, coretools.ErrNoMatches
	}
	if filter.OSType != "" {
		if err := checkToolsReleases(list, filter.OSType); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// FindExactTools returns only the tools that match the supplied version.
func FindExactTools(ctx context.Context, ss SimplestreamsFetcher, env environs.Environ, vers version.Number, osType string, arch string) (_ *coretools.Tools, err error) {
	logger.Debugf(ctx, "finding exact version %s", vers)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	filter := coretools.Filter{
		Number: vers,
		OSType: osType,
		Arch:   arch,
	}
	streams := PreferredStreams(&vers, env.Config().Development(), env.Config().AgentStream())
	logger.Debugf(ctx, "looking for agent binaries in streams %v", streams)
	availableTools, err := FindTools(ctx, ss, env, vers.Major, vers.Minor, streams, filter)
	if err != nil {
		return nil, err
	}
	if len(availableTools) != 1 {
		return nil, fmt.Errorf("expected one agent binary, got %d agent binaries", len(availableTools))
	}
	return availableTools[0], nil
}

// checkToolsReleases verifies that all the given possible tools are for the
// given OS osType.
func checkToolsReleases(toolsList coretools.List, release string) error {
	toolsReleases := toolsList.AllReleases()
	if len(toolsReleases) != 1 {
		return fmt.Errorf("expected single os type, got %v", toolsReleases)
	}
	if toolsReleases[0] != release {
		return fmt.Errorf("agent binary mismatch: expected os type %v, got %v", release, toolsReleases[0])
	}
	return nil
}

func isToolsError(err error) bool {
	switch err {
	case ErrNoTools, coretools.ErrNoMatches:
		return true
	}
	return false
}

func convertToolsError(err *error) {
	if isToolsError(*err) {
		*err = errors.NewNotFound(*err, "")
	}
}

var streamFallbacks = map[string][]string{
	ReleasedStream: {ReleasedStream},
	ProposedStream: {ProposedStream, ReleasedStream},
	DevelStream:    {DevelStream, ProposedStream, ReleasedStream},
	TestingStream:  {TestingStream, DevelStream, ProposedStream, ReleasedStream},
}

// PreferredStreams returns the tools streams that should be searched
// for tools, based on the required version, whether devel mode is
// required, and any user specified stream. The streams are in
// fallback order - if there are no matching tools in one stream the
// next should be checked.
func PreferredStreams(vers *version.Number, forceDevel bool, stream string) []string {
	// If the use has already nominated a specific stream, we'll use that.
	if stream != "" && stream != ReleasedStream {
		if fallbacks, ok := streamFallbacks[stream]; ok {
			return copyStrings(fallbacks)
		}
		return []string{stream}
	}
	// If we're not upgrading from a known version, we use the
	// currently running version.
	if vers == nil {
		vers = &jujuversion.Current
	}
	// Devel versions are alpha or beta etc as defined by the version tag.
	// The user can also force the use of devel streams via config.
	if forceDevel || jujuversion.IsDev(*vers) {
		return copyStrings(streamFallbacks[DevelStream])
	}
	return copyStrings(streamFallbacks[ReleasedStream])
}

func copyStrings(vals []string) []string {
	result := make([]string, len(vals))
	copy(result, vals)
	return result
}
