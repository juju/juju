// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
	"launchpad.net/loggo"
)

var logger = loggo.GetLogger("juju.environs.tools")

// FindTools returns a List containing all tools with a given
// major.minor version number available in the storages, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// The storages are queries in order - if *any* tools are present in a storage,
// *only* tools that storage are available for use.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func FindTools(storages []environs.StorageReader,
	majorVersion, minorVersion int, filter coretools.Filter) (list coretools.List, err error) {

	logger.Infof("reading tools with major.minor version %d.%d", majorVersion, minorVersion)
	defer convertToolsError(&err)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	if filter.Number != version.Zero {
		logger.Infof("filtering tools by version: %s", filter.Number.Major)
	}
	if filter.Series != "" {
		logger.Infof("filtering tools by series: %s", filter.Series)
	}
	if filter.Arch != "" {
		logger.Infof("filtering tools by architecture: %s", filter.Arch)
	}
	for _, storage := range storages {
		list, err = ReadList(storage, majorVersion, minorVersion)
		if err != ErrNoTools {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	list, err = list.Match(filter)
	if err != nil {
		return nil, err
	}
	if filter.Series != "" {
		if err := checkToolsSeries(list, filter.Series); err != nil {
			return nil, err
		}
	}
	return list, err
}

// FindBootstrapTools returns a ToolsList containing only those tools with
// which it would be reasonable to launch an environment's first machine, given the supplied constraints.
// If a specific agent version is not requested, all tools matching the current major.minor version are chosen.
func FindBootstrapTools(storages []environs.StorageReader,
	vers *version.Number, series string, arch *string, useDev bool) (list coretools.List, err error) {

	// Construct a tools filter.
	cliVersion := version.Current.Number
	filter := coretools.Filter{
		Series: series,
		Arch:   stringOrEmpty(arch),
	}
	if vers != nil {
		// If we already have an explicit agent version set, we're done.
		filter.Number = *vers
		return FindTools(storages, cliVersion.Major, cliVersion.Minor, filter)
	}
	if dev := cliVersion.IsDev() || useDev; !dev {
		logger.Infof("filtering tools by released version")
		filter.Released = true
	}
	return FindTools(storages, cliVersion.Major, cliVersion.Minor, filter)
}

func stringOrEmpty(pstr *string) string {
	if pstr == nil {
		return ""
	}
	return *pstr
}

// FindInstanceTools returns a ToolsList containing only those tools with which
// it would be reasonable to start a new instance, given the supplied series and arch.
func FindInstanceTools(storages []environs.StorageReader,
	vers version.Number, series string, arch *string) (list coretools.List, err error) {

	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	filter := coretools.Filter{
		Number: vers,
		Series: series,
		Arch:   stringOrEmpty(arch),
	}
	return FindTools(storages, vers.Major, vers.Minor, filter)
}

// FindExactTools returns only the tools that match the supplied version.
func FindExactTools(storages []environs.StorageReader,
	vers version.Number, series string, arch string) (t *coretools.Tools, err error) {

	logger.Infof("finding exact version %s", vers)
	// Construct a tools filter.
	// Discard all that are known to be irrelevant.
	filter := coretools.Filter{
		Number: vers,
		Series: series,
		Arch:   arch,
	}
	availaleTools, err := FindTools(storages, vers.Major, vers.Minor, filter)
	if err != nil {
		return nil, err
	}
	if len(availaleTools) != 1 {
		return nil, fmt.Errorf("expected one tools, got %d tools", len(availaleTools))
	}
	return availaleTools[0], nil
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
