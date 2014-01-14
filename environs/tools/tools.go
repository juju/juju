// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/errors"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.tools")

func makeToolsConstraint(cloudSpec simplestreams.CloudSpec, majorVersion, minorVersion int,
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
		toolsConstraint = NewVersionedToolsConstraint(filter.Number.String(),
			simplestreams.LookupParams{CloudSpec: cloudSpec})
	} else {
		toolsConstraint = NewGeneralToolsConstraint(majorVersion, minorVersion, filter.Released,
			simplestreams.LookupParams{CloudSpec: cloudSpec})
	}
	if filter.Arch != "" {
		toolsConstraint.Arches = []string{filter.Arch}
	} else {
		logger.Debugf("no architecture specified when finding tools, looking for any")
		toolsConstraint.Arches = []string{"amd64", "i386", "arm"}
	}
	// The old tools search allowed finding tools without needing to specify a series.
	// The simplestreams metadata is keyed off series, so series must be specified in
	// the search constraint. If no series is specified, we gather all the series from
	// lucid onwards and add those to the constraint.
	var seriesToSearch []string
	if filter.Series != "" {
		seriesToSearch = []string{filter.Series}
	} else {
		logger.Debugf("no series specified when finding tools, looking for any")
		seriesToSearch = simplestreams.SupportedSeries()
	}
	toolsConstraint.Series = seriesToSearch
	return toolsConstraint, nil
}

// Define some boolean parameter values.
const DoNotAllowRetry = false

// FindTools returns a List containing all tools with a given
// major.minor version number available in the cloud instance, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindTools(cloudInst environs.ConfigGetter, majorVersion, minorVersion int,
	filter coretools.Filter, allowRetry bool) (list coretools.List, err error) {

	var cloudSpec simplestreams.CloudSpec
	if inst, ok := cloudInst.(simplestreams.HasRegion); ok {
		if cloudSpec, err = inst.Region(); err != nil {
			return nil, err
		}
	}
	// If only one of region or endpoint is provided, that is a problem.
	if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
		return nil, fmt.Errorf("cannot find tools without a complete cloud configuration")
	}

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
	sources, err := GetMetadataSourcesWithRetries(cloudInst, allowRetry)
	if err != nil {
		return nil, err
	}
	return FindToolsForCloud(sources, cloudSpec, majorVersion, minorVersion, filter)
}

// FindToolsForCloud returns a List containing all tools with a given
// major.minor version number and cloudSpec, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindToolsForCloud(sources []simplestreams.DataSource, cloudSpec simplestreams.CloudSpec,
	majorVersion, minorVersion int, filter coretools.Filter) (list coretools.List, err error) {

	toolsConstraint, err := makeToolsConstraint(cloudSpec, majorVersion, minorVersion, filter)
	if err != nil {
		return nil, err
	}
	toolsMetadata, err := Fetch(sources, simplestreams.DefaultIndexPath, toolsConstraint, false)
	if err != nil {
		if errors.IsNotFoundError(err) {
			err = ErrNoTools
		}
		return nil, err
	}
	if len(toolsMetadata) == 0 {
		return nil, coretools.ErrNoMatches
	}
	list = make(coretools.List, len(toolsMetadata))
	for i, metadata := range toolsMetadata {
		list[i] = &coretools.Tools{
			Version: metadata.binary(),
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

// The following allows FindTools, as called by FindBootstrapTools, to be patched for testing.
var bootstrapFindTools = FindTools

type findtoolsfunc func(environs.ConfigGetter, int, int, coretools.Filter, bool) (coretools.List, error)

func TestingPatchBootstrapFindTools(stub findtoolsfunc) func() {
	origFunc := bootstrapFindTools
	bootstrapFindTools = stub
	return func() {
		bootstrapFindTools = origFunc
	}
}

// BootstrapToolsParams contains parameters for FindBootstrapTools
type BootstrapToolsParams struct {
	Version    *version.Number
	Arch       *string
	Series     string
	AllowRetry bool
}

// FindBootstrapTools returns a ToolsList containing only those tools with
// which it would be reasonable to launch an environment's first machine, given the supplied constraints.
// If a specific agent version is not requested, all tools matching the current major.minor version are chosen.
func FindBootstrapTools(cloudInst environs.ConfigGetter, params BootstrapToolsParams) (list coretools.List, err error) {
	// Construct a tools filter.
	cfg := cloudInst.Config()
	cliVersion := version.Current.Number
	filter := coretools.Filter{
		Series: params.Series,
		Arch:   stringOrEmpty(params.Arch),
	}
	if params.Version != nil {
		// If we already have an explicit agent version set, we're done.
		filter.Number = *params.Version
		return bootstrapFindTools(cloudInst, cliVersion.Major, cliVersion.Minor, filter, params.AllowRetry)
	}
	if dev := cliVersion.IsDev() || cfg.Development(); !dev {
		logger.Infof("filtering tools by released version")
		filter.Released = true
	}
	return bootstrapFindTools(cloudInst, cliVersion.Major, cliVersion.Minor, filter, params.AllowRetry)
}

func stringOrEmpty(pstr *string) string {
	if pstr == nil {
		return ""
	}
	return *pstr
}

// FindInstanceTools returns a ToolsList containing only those tools with which
// it would be reasonable to start a new instance, given the supplied series and arch.
func FindInstanceTools(cloudInst environs.ConfigGetter,
	vers version.Number, series string, arch *string) (list coretools.List, err error) {

	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	filter := coretools.Filter{
		Number: vers,
		Series: series,
		Arch:   stringOrEmpty(arch),
	}
	return FindTools(cloudInst, vers.Major, vers.Minor, filter, DoNotAllowRetry)
}

// FindExactTools returns only the tools that match the supplied version.
func FindExactTools(cloudInst environs.ConfigGetter,
	vers version.Number, series string, arch string) (t *coretools.Tools, err error) {

	logger.Infof("finding exact version %s", vers)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	filter := coretools.Filter{
		Number: vers,
		Series: series,
		Arch:   arch,
	}
	availableTools, err := FindTools(cloudInst, vers.Major, vers.Minor, filter, DoNotAllowRetry)
	if err != nil {
		return nil, err
	}
	if len(availableTools) != 1 {
		return nil, fmt.Errorf("expected one tools, got %d tools", len(availableTools))
	}
	return availableTools[0], nil
}

// CheckToolsSeries verifies that all the given possible tools are for the
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
		*err = errors.NewNotFoundError(*err, "")
	}
}
