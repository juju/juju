// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname

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
// hostname into its constituent parts.
// The target field indicates what kind of
// hostname was parsed which will indicate
// that some fields are empty.
type Info struct {
	target    HostnameTarget
	modelUUID string
	machine   int
	unit      int
	app       string
	container string
}

// Unit returns the unit name.
func (i Info) Unit() (string, bool) {
	return fmt.Sprintf("%s/%d", i.app, i.unit), i.target != MachineTarget
}

// Container returns the container name
// and a bool to indicate if a container
// is valid for the target type.
func (i Info) Container() (string, bool) {
	return i.container, i.target == ContainerTarget
}

// ModelUUID returns the model UUID.
func (i Info) ModelUUID() string {
	return i.modelUUID
}

// Machine returns the machine number.
func (i Info) Machine() (int, bool) {
	return i.machine, i.target == MachineTarget
}

// HostnameTarget returns an enum value indicating the
// target of the hostname e.g. container, machine, etc.
func (i Info) Target() HostnameTarget {
	return i.target
}

var (
	// hostnameMatcher parses a hostname of the following formats:
	// Machine: 1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	// Unit: 1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	// Container: charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	// The regular expression doesn't validate the components of the
	// hostname, it only extracts them for validation separately.
	// I.e. the extracted UUID may be invalid.
	hostnameMatcher = regexp.MustCompile(`^(?:(?<containername>[a-zA-Z0-9-]+)\.)?(?<unitnumber>\d+)\.(?:(?<appname>[a-zA-Z0-9-]+)\.)?(?<modeluuid>[0-9a-fA-F-]+)\.(?<domain>[a-zA-Z0-9.-]+)$`)
)

// Parse parses a virtual Juju hostname
// that references entities like machines, units
// and containers.
func Parse(hostname string) (Info, error) {
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

	// Validate the components where appropriate.
	if !names.IsValidModel(result["modeluuid"]) {
		return Info{}, errors.Errorf("invalid model UUID: %q", result["modeluuid"])
	}
	if result["appname"] != "" && !names.IsValidApplication(result["appname"]) {
		return Info{}, errors.Errorf("invalid application name: %q", result["appname"])
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
		res.target = UnitTarget
	} else {
		res.target = MachineTarget
		res.machine = unitNumber
		res.unit = 0
	}

	return res, nil
}
