// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostname

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
)

const (
	// ErrNoMatch indicates that the hostname could not be parsed.
	ErrNoMatch = errors.ConstError("could not parse hostname")
)

// HostnameTarget defines what kind of infrastructure the user is targeting.
type HostnameTarget int

const (
	// MachineTarget defines a machine as the target.
	MachineTarget HostnameTarget = iota

	// UnitTarget defines a unit (K8s or machine)
	// as the target.
	UnitTarget

	// ContainerTarget defines a container within a
	// unit as the target.
	ContainerTarget
)

// Info returns a breakdown of a virtual
// hostname into it's constituent parts.
// The target field indicates what kind of
// hostname was parsed which will indicate
// that some fields are empty.
type Info struct {
	modelUUID string
	machine   int
	unit      int
	app       string
	container string
	target    HostnameTarget
}

// Unit returns the unit name, appropriate
// for use in state methods.
func (i Info) Unit() string {
	return fmt.Sprintf("%s/%d", i.app, i.unit)
}

// App returns the application name.
func (i Info) Application() string {
	return i.app
}

// Container returns the container name.
func (i Info) Container() string {
	return i.container
}

// ModelUUID returns the model UUID.
func (i Info) ModelUUID() string {
	return i.modelUUID
}

// Machine returns the machine number.
func (i Info) Machine() int {
	return i.machine
}

// HostnameTarget returns an enum value indicating the
// target of the hostname e.g. container, machine, etc.
func (i Info) Target() HostnameTarget {
	return i.target
}

var (
	// hostnameMatcher parses a hostname of various formats including,
	// Machine: 1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	// Unit: 1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	// Container: charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	hostnameMatcher = regexp.MustCompile(`^(?:(?<containername>[a-zA-Z0-9-]+)\.)?(?<unitnumber>\d+)\.(?:(?<appname>[a-zA-Z0-9-]+)\.)?(?<modeluuid>[0-9a-fA-F-]+)\.(?<domain>[a-zA-Z0-9.-]+)$`)
)

// ParseHostname parses a virtual Juju hostname
// that references entities like machines, units
// and containers.
func ParseHostname(hostname string) (Info, error) {
	match := hostnameMatcher.FindStringSubmatch(hostname)
	if match == nil {
		return Info{}, ErrNoMatch
	}
	result := make(map[string]string)
	for i, name := range hostnameMatcher.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	if !names.IsValidModel(result["modeluuid"]) {
		return Info{}, errors.New("invalid model UUID")
	}
	// unit number and machine number come from the same matching group.
	unitNumber, err := strconv.Atoi(result["unitnumber"])
	if err != nil {
		return Info{}, errors.Annotatef(err, "failed to parse unit/machine number")
	}

	res := Info{}
	res.modelUUID = result["modeluuid"]
	res.container = result["containername"]
	res.app = result["appname"]
	res.unit = unitNumber

	if res.container != "" {
		res.target = ContainerTarget
	} else if res.app != "" {
		if !names.IsValidApplication(res.app) {
			return Info{}, errors.New("invalid application name")
		}
		res.target = UnitTarget
	} else {
		res.target = MachineTarget
		res.machine = unitNumber
		res.unit = 0
	}

	return res, nil
}
