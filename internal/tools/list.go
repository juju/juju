// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/semversion"
)

// List holds tools available in an environment. The order of tools within
// a List is not significant.
type List []*Tools

var ErrNoMatches = errors.New("no matching agent binaries available")

// String returns the versions of the tools in src, separated by semicolons.
func (src List) String() string {
	names := make([]string, len(src))
	for i, tools := range src {
		names[i] = tools.Version.String()
	}
	return strings.Join(names, ";")
}

// AllReleases returns all os types for which some tools in src were built.
func (src List) AllReleases() []string {
	return src.collect(func(tools *Tools) string {
		return tools.Version.Release
	})
}

// OneRelease returns the single os type for which all tools in src were built.
func (src List) OneRelease() string {
	release := src.AllReleases()
	if len(release) != 1 {
		panic(fmt.Errorf("should have gotten tools for one os type, got %v", release))
	}
	return release[0]
}

// OneArch returns a single architecture for all tools in src,
// or an error if there's more than one arch (or none) present.
func (src List) OneArch() (string, error) {
	allArches := src.collect(func(tools *Tools) string {
		return tools.Version.Arch
	})
	if len(allArches) == 0 {
		return "", errors.New("tools list is empty")
	}
	if len(allArches) != 1 {
		return "", errors.Errorf("more than one agent arch present: %v", allArches)
	}
	return allArches[0], nil
}

// collect calls f on all values in src and returns an alphabetically
// ordered list of the returned results without duplicates.
func (src List) collect(f func(*Tools) string) []string {
	seen := make(set.Strings)
	for _, tools := range src {
		seen.Add(f(tools))
	}
	return seen.SortedValues()
}

// URLs returns download URLs for the tools in src, keyed by binary
// version. Each version can have more than one URL.
func (src List) URLs() map[semversion.Binary][]string {
	result := map[semversion.Binary][]string{}
	for _, tools := range src {
		result[tools.Version] = append(result[tools.Version], tools.URL)
	}
	return result
}

// HasVersion instance store an agent version.
type HasVersion interface {
	// AgentVersion returns the agent version.
	AgentVersion() semversion.Number
}

// Versions holds instances of HasVersion.
type Versions []HasVersion

// Newest returns the greatest version in src, and the tools with that version.
func (src List) Newest() (semversion.Number, List) {
	var result List
	var best semversion.Number
	for _, tools := range src {
		if best.Compare(tools.Version.Number) < 0 {
			// Found new best number; reset result list.
			best = tools.Version.Number
			result = append(result[:0], tools)
		} else if tools.Version.Number == best {
			result = append(result, tools)
		}
	}
	return best, result
}

// Newest returns the greatest version in src, and the instances with that version.
func (src Versions) Newest() (semversion.Number, Versions) {
	var result Versions
	var best semversion.Number
	for _, agent := range src {
		if best.Compare(agent.AgentVersion()) < 0 {
			// Found new best number; reset result list.
			best = agent.AgentVersion()
			result = append(result[:0], agent)
		} else if agent.AgentVersion() == best {
			result = append(result, agent)
		}
	}
	return best, result
}

// NewestCompatible returns the most recent version compatible with
// base, i.e. with the same major number and greater or
// equal minor, patch and build numbers.
func (src Versions) NewestCompatible(base semversion.Number, allowDevBuilds bool) (newest semversion.Number, found bool) {
	newest = base
	found = false
	for _, agent := range src {
		agentVersion := agent.AgentVersion()
		if newest == agentVersion {
			found = true
		} else if newest.Compare(agentVersion) < 0 &&
			agentVersion.Major == newest.Major &&
			agentVersion.Minor == newest.Minor &&
			(allowDevBuilds || agentVersion.Build == 0) {
			newest = agentVersion
			found = true
		}
	}
	return newest, found
}

// Exclude returns the tools in src that are not in excluded.
func (src List) Exclude(excluded List) List {
	ignore := make(map[semversion.Binary]bool, len(excluded))
	for _, tool := range excluded {
		ignore[tool.Version] = true
	}
	var result List
	for _, tool := range src {
		if !ignore[tool.Version] {
			result = append(result, tool)
		}
	}
	return result
}

// Match returns a List, derived from src, containing only those tools that
// match the supplied Filter. If no tools match, it returns ErrNoMatches.
func (src List) Match(f Filter) (List, error) {
	var result List
	for _, tools := range src {
		if f.match(tools) {
			result = append(result, tools)
		}
	}
	if len(result) == 0 {
		return nil, ErrNoMatches
	}
	return result, nil
}

// Match returns a List, derived from src, containing only those instances that
// match the supplied Filter. If no tools match, it returns ErrNoMatches.
func (src Versions) Match(f Filter) (Versions, error) {
	var result Versions
	for _, agent := range src {
		if f.match(agent) {
			result = append(result, agent)
		}
	}
	if len(result) == 0 {
		return nil, ErrNoMatches
	}
	return result, nil
}

func (l List) Len() int { return len(l) }

func (l List) Swap(i, j int) { l[i], l[j] = l[j], l[i] }

func (l List) Less(i, j int) bool { return l[i].Version.String() < l[j].Version.String() }

// Filter holds criteria for choosing tools.
type Filter struct {
	// Number, if non-zero, causes the filter to match only tools with
	// that exact version number.
	Number semversion.Number

	// OSType, if not empty, causes the filter to match only tools with
	// that os type.
	OSType string

	// Arch, if not empty, causes the filter to match only tools with
	// that architecture.
	Arch string
}

// match returns true if the supplied tools match f.
func (f Filter) match(agent HasVersion) bool {
	if f.Number != semversion.Zero && agent.AgentVersion() != f.Number {
		return false
	}
	tools, ok := agent.(*Tools)
	if !ok {
		return true
	}
	if f.OSType != "" && tools.Version.Release != f.OSType {
		return false
	}
	if f.Arch != "" && tools.Version.Arch != f.Arch {
		return false
	}
	return true
}
