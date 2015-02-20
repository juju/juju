// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) At some point we should consider moving the
// initsystems package and portions of the service package into another
// repo (e.g. utils).

package initsystems

// TODO(ericsnow) Store the local init system in an exported var.

import (
	"runtime"
	"strings"

	"github.com/juju/errors"
)

// InitSystem represents the functionality provided by an init system.
// It encompasses all init services on the host, rather than just juju-
// managed ones.
type InitSystem interface {
	ConfHandler

	// List gathers the names of all enabled services in the init system
	// and returns them. If any names are passed as arguments then the
	// result will be limited to those names. Otherwise all known
	// service names are returned.
	List(include ...string) ([]string, error)

	// Start causes the named service to be started. If it is already
	// started then errors.AlreadyExists is returned. If the service has
	// not been enabled then errors.NotFound is returned.
	Start(name string) error

	// Stop causes the named service to be stopped. If it is already
	// stopped then errors.NotFound is returned. If the service has
	// not been enabled then errors.NotFound is returned.
	Stop(name string) error

	// Enable adds a new service to the init system with the given name.
	// The conf file at the provided filename is used for the new
	// service. If a service with that name is already enabled then
	// errors.AlreadyExists is returned. The file will be deserialized
	// and validated before the service is enabled.
	Enable(name, filename string) error

	// Disable removes the named service from the init system. If it is
	// not already enabled then errors.NotFound is returned.
	Disable(name string) error

	// IsEnabled determines whether or not the named service is enabled.
	IsEnabled(name string) (bool, error)

	// Check determines whether or not the named service is enabled
	// and matches the conf at the filename.
	Check(name, filename string) (bool, error)

	// Info gathers information about the named service and returns it.
	// If the service is not enabled then errors.NotFound is returned.
	Info(name string) (ServiceInfo, error)

	// Conf composes a Conf for the named service and returns it.
	// If the service is not enabled then errors.NotFound is returned.
	Conf(name string) (Conf, error)
}

type newInitSystemFunc func(string) InitSystem

// InitSystemDefinition holds info about a single InitSystem
// implementation. This information may be used to decide if the
// implementation is appropriate for some use case, e.g. in
// DiscoverInitSystem.
type InitSystemDefinition struct {
	// Name is a unique identier for the init system.
	Name string

	// OSNames is the a list of OS names supported by the init system,
	// The values will be compared against runtime.GOOS.
	OSNames []string

	// Executables is the list of absolute paths to executables under
	// which the init system may be running. On linux
	Executables []string

	// New is the function to use to create a new instance.
	New newInitSystemFunc
}

func (isd InitSystemDefinition) supportedOS(osName string) bool {
	for _, supported := range isd.OSNames {
		if supported == osName {
			return true
		}
		if strings.HasPrefix(supported, "!") && supported[1:] != osName {
			return true
		}
	}
	return false
}

func (isd InitSystemDefinition) supportedExecutable(executable string) bool {
	for _, supported := range isd.Executables {
		if supported == executable {
			return true
		}
	}
	return false
}

var registeredImplementations = map[string]InitSystemDefinition{}

// Register adds an InitSystem implementation to the registry. If the
// provided name is already registered then errors.AlreadyExists is
// returned.
func Register(name string, definition InitSystemDefinition) error {
	if _, ok := registeredImplementations[name]; ok {
		// TODO(ericsnow) return nil if they are the same?
		return errors.AlreadyExistsf("init system %q", name)
	}
	registeredImplementations[name] = definition
	return nil
}

// NewInitSystem returns an InitSystem implementation based on the
// provided name. If the name is unrecognized then nil is returned.
func NewInitSystem(name string) InitSystem {
	definition, ok := registeredImplementations[name]
	if !ok {
		return nil
	}

	return definition.New(name)
}

// TODO(ericsnow) Support discovering init system on remote host.

// DiscoverInitSystem determines the name of the init system in use on
// the local host and returns it. This involves checking all registered
// InitSystem implementations for a match on the operating system and
// on the currently running init system executable. If no matches are
// found then the empty string is returned.
func DiscoverInitSystem() string {
	return discoverInitSystem(
		filterByOS,
		filterByExecutable,
	)
}

type candidates map[string]InitSystemDefinition

type initSystemFilter func(candidates) candidates

func discoverInitSystem(filters ...initSystemFilter) string {
	candidates := registeredImplementations
	for _, filter := range filters {
		candidates = filter(candidates)
		if len(candidates) == 0 {
			return ""
		}
	}
	// We are guaranteed at least one candidate at this point.
	// TODO(ericsnow) Fail if more than one candidate left?
	for name := range candidates {
		return name
	}
	return ""
}

func filterByOS(values candidates) candidates {
	osName := runtime.GOOS

	results := candidates{}
	for name, definition := range values {
		if definition.supportedOS(osName) {
			results[name] = definition
		}
	}
	return results
}

func filterByExecutable(values candidates) candidates {
	executable, err := findInitExecutable()
	if err != nil {
		return nil
	}

	results := candidates{}
	for name, definition := range values {
		if definition.supportedExecutable(executable) {
			results[name] = definition
		}
	}
	return results
}
