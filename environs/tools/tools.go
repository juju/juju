// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/utils/arch"
	"github.com/juju/version"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.environs.tools")

func makeToolsConstraint(cloudSpec simplestreams.CloudSpec, stream string, majorVersion, minorVersion int,
	filter coretools.Filter) (*ToolsConstraint, error) {

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
		logger.Tracef("no architecture specified when finding agent binaries, looking for any")
		toolsConstraint.Arches = arch.AllSupportedArches
	}
	// The old tools search allowed finding tools without needing to specify a series.
	// The simplestreams metadata is keyed off series, so series must be specified in
	// the search constraint. If no series is specified, we gather all the series from
	// lucid onwards and add those to the constraint.
	var seriesToSearch []string
	if filter.Series != "" {
		seriesToSearch = []string{filter.Series}
	} else {
		seriesToSearch = series.SupportedSeries()
		logger.Tracef("no series specified when finding agent binaries, looking for %v", seriesToSearch)
	}
	toolsConstraint.Series = seriesToSearch
	return toolsConstraint, nil
}

// HasAgentMirror is an optional interface that an Environ may
// implement to support agent/tools mirror lookup.
//
// TODO(axw) 2016-04-11 #1568715
// This exists only because we currently lack
// image simplestreams usable by the new Azure
// Resource Manager provider. When we have that,
// we can use "HasRegion" everywhere.
type HasAgentMirror interface {
	// AgentMirror returns the CloudSpec to use for looking up agent
	// binaries.
	AgentMirror() (simplestreams.CloudSpec, error)
}

// FindTools returns a List containing all tools in the given stream, with a given
// major.minor version number available in the cloud instance, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindTools(env environs.BootstrapEnviron, majorVersion, minorVersion int, streams []string, filter coretools.Filter) (_ coretools.List, err error) {
	var cloudSpec simplestreams.CloudSpec
	switch env := env.(type) {
	case simplestreams.HasRegion:
		if cloudSpec, err = env.Region(); err != nil {
			return nil, err
		}
	case HasAgentMirror:
		if cloudSpec, err = env.AgentMirror(); err != nil {
			return nil, err
		}
	}
	// If only one of region or endpoint is provided, that is a problem.
	if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
		return nil, errors.New("cannot find agent binaries without a complete cloud configuration")
	}

	logger.Debugf("finding agent binaries in stream: %q", strings.Join(streams, ", "))
	if minorVersion >= 0 {
		logger.Debugf("reading agent binaries with major.minor version %d.%d", majorVersion, minorVersion)
	} else {
		logger.Debugf("reading agent binaries with major version %d", majorVersion)
	}
	defer convertToolsError(&err)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	if filter.Number != version.Zero {
		logger.Debugf("filtering agent binaries by version: %s", filter.Number)
	}
	if filter.Series != "" {
		logger.Debugf("filtering agent binaries by series: %s", filter.Series)
	}
	if filter.Arch != "" {
		logger.Debugf("filtering agent binaries by architecture: %s", filter.Arch)
	}
	sources, err := GetMetadataSources(env)
	if err != nil {
		return nil, err
	}
	return FindToolsForCloud(sources, cloudSpec, streams, majorVersion, minorVersion, filter)
}

// FindToolsForCloud returns a List containing all tools in the given streams, with a given
// major.minor version number and cloudSpec, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindToolsForCloud(sources []simplestreams.DataSource, cloudSpec simplestreams.CloudSpec, streams []string,
	majorVersion, minorVersion int, filter coretools.Filter) (coretools.List, error) {
	var list coretools.List
	noToolsCount := 0
	seenBinary := make(map[version.Binary]bool)
	for _, stream := range streams {
		toolsConstraint, err := makeToolsConstraint(cloudSpec, stream, majorVersion, minorVersion, filter)
		if err != nil {
			return nil, err
		}
		toolsMetadata, _, err := Fetch(sources, toolsConstraint)
		if errors.IsNotFound(err) {
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
	if filter.Series != "" {
		if err := checkToolsSeries(list, filter.Series); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// FindExactTools returns only the tools that match the supplied version.
func FindExactTools(env environs.Environ, vers version.Number, series string, arch string) (_ *coretools.Tools, err error) {
	logger.Debugf("finding exact version %s", vers)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	filter := coretools.Filter{
		Number: vers,
		Series: series,
		Arch:   arch,
	}
	streams := PreferredStreams(&vers, env.Config().Development(), env.Config().AgentStream())
	logger.Debugf("looking for agent binaries in streams %v", streams)
	availableTools, err := FindTools(env, vers.Major, vers.Minor, streams, filter)
	if err != nil {
		return nil, err
	}
	if len(availableTools) != 1 {
		return nil, fmt.Errorf("expected one agent binary, got %d agent binaries", len(availableTools))
	}
	return availableTools[0], nil
}

// checkToolsSeries verifies that all the given possible tools are for the
// given OS series.
func checkToolsSeries(toolsList coretools.List, series string) error {
	toolsSeries := toolsList.AllSeries()
	if len(toolsSeries) != 1 {
		return fmt.Errorf("expected single series, got %v", toolsSeries)
	}
	if toolsSeries[0] != series {
		return fmt.Errorf("agent binary mismatch: expected series %v, got %v", series, toolsSeries[0])
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
