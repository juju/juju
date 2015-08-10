// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju/arch"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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
		logger.Tracef("no architecture specified when finding tools, looking for any")
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
		seriesToSearch = version.SupportedSeries()
		logger.Tracef("no series specified when finding tools, looking for %v", seriesToSearch)
	}
	toolsConstraint.Series = seriesToSearch
	return toolsConstraint, nil
}

// Define some boolean parameter values.
const DoNotAllowRetry = false

// FindTools returns a List containing all tools in the given stream, with a given
// major.minor version number available in the cloud instance, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindTools(env environs.Environ, majorVersion, minorVersion int, stream string,
	filter coretools.Filter) (list coretools.List, err error) {

	var cloudSpec simplestreams.CloudSpec
	if inst, ok := env.(simplestreams.HasRegion); ok {
		if cloudSpec, err = inst.Region(); err != nil {
			return nil, err
		}
	}
	// If only one of region or endpoint is provided, that is a problem.
	if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
		return nil, fmt.Errorf("cannot find tools without a complete cloud configuration")
	}

	logger.Infof("findng tools in stream %q", stream)
	if minorVersion >= 0 {
		logger.Infof("reading tools with major.minor version %d.%d", majorVersion, minorVersion)
	} else {
		logger.Infof("reading tools with major version %d", majorVersion)
	}
	defer convertToolsError(&err)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	if filter.Number != version.Zero {
		logger.Infof("filtering tools by version: %s", filter.Number)
	}
	if filter.Series != "" {
		logger.Infof("filtering tools by series: %s", filter.Series)
	}
	if filter.Arch != "" {
		logger.Infof("filtering tools by architecture: %s", filter.Arch)
	}
	sources, err := GetMetadataSources(env)
	if err != nil {
		return nil, err
	}
	return FindToolsForCloud(sources, cloudSpec, stream, majorVersion, minorVersion, filter)
}

// FindToolsForCloud returns a List containing all tools in the given stream, with a given
// major.minor version number and cloudSpec, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindToolsForCloud(sources []simplestreams.DataSource, cloudSpec simplestreams.CloudSpec, stream string,
	majorVersion, minorVersion int, filter coretools.Filter) (list coretools.List, err error) {

	toolsConstraint, err := makeToolsConstraint(cloudSpec, stream, majorVersion, minorVersion, filter)
	if err != nil {
		return nil, err
	}
	toolsMetadata, _, err := Fetch(sources, toolsConstraint, false)
	if err != nil {
		if errors.IsNotFound(err) {
			err = ErrNoTools
		}
		return nil, err
	}
	if len(toolsMetadata) == 0 {
		return nil, coretools.ErrNoMatches
	}
	list = make(coretools.List, len(toolsMetadata))
	for i, metadata := range toolsMetadata {
		binary, err := metadata.binary()
		if err != nil {
			return nil, errors.Trace(err)
		}
		list[i] = &coretools.Tools{
			Version: binary,
			URL:     metadata.FullPath,
			Size:    metadata.Size,
			SHA256:  metadata.SHA256,
		}
	}
	if filter.Series != "" {
		if err := checkToolsSeries(list, filter.Series); err != nil {
			return nil, err
		}
	}
	return list, err
}

func stringOrEmpty(pstr *string) string {
	if pstr == nil {
		return ""
	}
	return *pstr
}

// FindExactTools returns only the tools that match the supplied version.
func FindExactTools(env environs.Environ, vers version.Number, series string, arch string) (t *coretools.Tools, err error) {
	logger.Infof("finding exact version %s", vers)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	filter := coretools.Filter{
		Number: vers,
		Series: series,
		Arch:   arch,
	}
	stream := PreferredStream(&vers, env.Config().Development(), env.Config().AgentStream())
	logger.Infof("looking for tools in stream %q", stream)
	availableTools, err := FindTools(env, vers.Major, vers.Minor, stream, filter)
	if err != nil {
		return nil, err
	}
	if len(availableTools) != 1 {
		return nil, fmt.Errorf("expected one tools, got %d tools", len(availableTools))
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
		return fmt.Errorf("tools mismatch: expected series %v, got %v", series, toolsSeries[0])
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

// PreferredStream returns the tools stream used to search for tools, based
// on the required version, whether devel mode is required, and any user specified stream.
func PreferredStream(vers *version.Number, forceDevel bool, stream string) string {
	// If the use has already nominated a specific stream, we'll use that.
	if stream != "" && stream != ReleasedStream {
		return stream
	}
	// If we're not upgrading from a known version, we use the
	// currently running version.
	if vers == nil {
		vers = &version.Current.Number
	}
	// Devel versions are alpha or beta etc as defined by the version tag.
	// The user can also force the use of devel streams via config.
	if forceDevel || vers.IsDev() {
		return DevelStream
	}
	return ReleasedStream
}
