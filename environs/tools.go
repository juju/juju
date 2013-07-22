// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/version"
)

// FindAvailableTools returns a tools.List containing all tools with a given
// major version number available in the environment.
// If *any* tools are present in private storage, *only* tools from private
// storage are available.
// If *no* tools are present in private storage, *only* tools from public
// storage are available.
// If no *available* tools have the supplied major version number, the function
// returns a *NotFoundError.
func FindAvailableTools(environ Environ, majorVersion int) (list tools.List, err error) {
	log.Infof("environs: reading tools with major version %d", majorVersion)
	defer convertToolsError(&err)
	list, err = tools.ReadList(environ.Storage(), majorVersion)
	if err == tools.ErrNoTools {
		log.Infof("environs: falling back to public bucket")
		list, err = tools.ReadList(environ.PublicStorage(), majorVersion)
	}
	return list, err
}

// FindBootstrapTools returns a ToolsList containing only those tools with
// which it would be reasonable to launch an environment's first machine,
// given the supplied constraints.
// If the environment was not already configured to use a specific agent
// version, the newest available version will be chosen and set in the
// environment's configuration.
func FindBootstrapTools(environ Environ, cons constraints.Value) (list tools.List, err error) {
	defer convertToolsError(&err)
	// Collect all possible compatible tools.
	cliVersion := version.Current.Number
	if list, err = FindAvailableTools(environ, cliVersion.Major); err != nil {
		return nil, err
	}

	// Discard all that are known to be irrelevant.
	cfg := environ.Config()
	series := cfg.DefaultSeries()
	log.Infof("environs: filtering tools by series: %s", series)
	filter := tools.Filter{Series: series}
	if cons.Arch != nil && *cons.Arch != "" {
		log.Infof("environs: filtering tools by architecture: %s", *cons.Arch)
		filter.Arch = *cons.Arch
	}
	if agentVersion, ok := cfg.AgentVersion(); ok {
		// If we already have an explicit agent version set, we're done.
		log.Infof("environs: filtering tools by version: %s", agentVersion)
		filter.Number = agentVersion
		return list.Match(filter)
	}
	if dev := cliVersion.IsDev() || cfg.Development(); !dev {
		log.Infof("environs: filtering tools by released version")
		filter.Released = true
	}
	if list, err = list.Match(filter); err != nil {
		return nil, err
	}

	// We probably still have a mix of versions available; discard older ones
	// and update environment configuration to use only those remaining.
	agentVersion, list := list.Newest()
	log.Infof("environs: picked newest version: %s", agentVersion)
	cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": agentVersion.String(),
	})
	if err == nil {
		err = environ.SetConfig(cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update environment configuration: %v", err)
	}
	return list, nil
}

// FindInstanceTools returns a ToolsList containing only those tools with which
// it would be reasonable to start a new instance, given the supplied series and
// constraints.
// It is an error to call it with an environment not already configured to use
// a specific agent version.
func FindInstanceTools(environ Environ, series string, cons constraints.Value) (list tools.List, err error) {
	defer convertToolsError(&err)
	// Collect all possible compatible tools.
	agentVersion, ok := environ.Config().AgentVersion()
	if !ok {
		return nil, fmt.Errorf("no agent version set in environment configuration")
	}
	if list, err = FindAvailableTools(environ, agentVersion.Major); err != nil {
		return nil, err
	}

	// Discard all that are known to be irrelevant.
	log.Infof("environs: filtering tools by version: %s", agentVersion)
	log.Infof("environs: filtering tools by series: %s", series)
	filter := tools.Filter{
		Number: agentVersion,
		Series: series,
	}
	if cons.Arch != nil && *cons.Arch != "" {
		log.Infof("environs: filtering tools by architecture: %s", *cons.Arch)
		filter.Arch = *cons.Arch
	}
	return list.Match(filter)
}

// FindExactTools returns only the tools that match the supplied version.
// TODO(fwereade) this should not exist: it's used by cmd/jujud/Upgrader,
// which needs to run on every agent and must absolutely *not* in general
// have access to an Environ.
func FindExactTools(environ Environ, vers version.Binary) (t *tools.Tools, err error) {
	defer convertToolsError(&err)
	list, err := FindAvailableTools(environ, vers.Major)
	if err != nil {
		return nil, err
	}
	log.Infof("environs: finding exact version %s", vers)
	list, err = list.Match(tools.Filter{
		Number: vers.Number,
		Series: vers.Series,
		Arch:   vers.Arch,
	})
	if err != nil {
		return nil, err
	}
	return list[0], nil
}

// CheckToolsSeries verifies that all the given possible tools are for the
// given OS series.
func CheckToolsSeries(tools tools.List, series string) error {
	toolsSeries := tools.Series()
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
	case tools.ErrNoTools, tools.ErrNoMatches:
		return true
	}
	return false
}

func convertToolsError(err *error) {
	if isToolsError(*err) {
		*err = &errors.NotFoundError{*err, ""}
	}
}
