// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
)

const (
	// ErrNoMatch indicates that the hostname could not be parsed.
	ErrNoMatch = errors.ConstError("could not parse hostname")
	Domain     = "juju.local"
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

// NewInfoMachineTarget returns a new Info struct for a machine target.
func NewInfoMachineTarget(modelUUID string, machine int) (Info, error) {
	if !names.IsValidModel(modelUUID) {
		return Info{}, errors.Errorf("invalid model UUID: %q", modelUUID)
	}
	if !names.IsValidMachine(fmt.Sprint(machine)) {
		return Info{}, errors.Errorf("invalid machine number: %d", machine)
	}
	return newInfo(MachineTarget, modelUUID, machine, "", ""), nil
}

// NewInfoUnitTarget returns a new Info struct for a unit target.
func NewInfoUnitTarget(modelUUID string, unit string) (Info, error) {
	if !names.IsValidModel(modelUUID) {
		return Info{}, errors.Errorf("invalid model UUID: %q", modelUUID)
	}
	if !names.IsValidUnit(unit) {
		return Info{}, errors.Errorf("invalid unit name: %s", unit)
	}
	return newInfo(UnitTarget, modelUUID, 0, unit, ""), nil
}

// NewInfoContainerTarget returns a new Info struct for a container target.
func NewInfoContainerTarget(modelUUID string, unit string, container string) (Info, error) {
	if !names.IsValidModel(modelUUID) {
		return Info{}, errors.Errorf("invalid model UUID: %q", modelUUID)
	}
	if !names.IsValidUnit(unit) {
		return Info{}, errors.Errorf("invalid unit name: %s", unit)
	}
	return newInfo(ContainerTarget, modelUUID, 0, unit, container), nil
}

// newInfo returns a new Info struct for the given target.
func newInfo(target HostnameTarget, modelUUID string, machine int, unit string, container string) Info {
	info := Info{}
	switch target {
	case MachineTarget:
		info.target = MachineTarget
		info.modelUUID = modelUUID
		info.machine = machine
	case UnitTarget:
		info.target = UnitTarget
		info.modelUUID = modelUUID
		info.unit = unit
	case ContainerTarget:
		info.target = ContainerTarget
		info.modelUUID = modelUUID
		info.unit = unit
		info.container = container
	}
	return info
}

// Info returns a breakdown of a virtual
// hostname into its constituent parts.
// The target field indicates what kind of
// hostname was parsed which will indicate
// that some fields are empty.
type Info struct {
	target    HostnameTarget
	modelUUID string
	machine   int
	unit      string
	container string
}

// Unit returns the unit name.
func (i Info) Unit() (string, bool) {
	return i.unit, i.target != MachineTarget
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

// String returns the virtual hostname stringified.
func (i Info) String() string {
	switch i.target {
	case MachineTarget:
		return fmt.Sprintf("%d.%s.%s", i.machine, i.modelUUID, Domain)
	case UnitTarget:
		parts := strings.Split(i.unit, "/")
		if len(parts) != 2 {
			panic("invalid unit name") // this shouldn't happen because we have validated the unit in the constructor.
		}
		return fmt.Sprintf("%s.%s.%s.%s", parts[1], parts[0], i.modelUUID, Domain)
	case ContainerTarget:
		parts := strings.Split(i.unit, "/")
		if len(parts) != 2 {
			panic("invalid unit name") // this shouldn't happen because we have validated the unit in the constructor.
		}
		return fmt.Sprintf("%s.%s.%s.%s.%s", i.container, parts[1], parts[0], i.modelUUID, Domain)
	default:
		return "unknown"
	}
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
	appName := result["appname"]
	res.modelUUID = result["modeluuid"]
	res.container = result["containername"]
	res.unit = fmt.Sprintf("%s/%d", appName, unitNumber)
	if res.container != "" {
		res.target = ContainerTarget
	} else if appName != "" {
		res.target = UnitTarget
	} else {
		res.target = MachineTarget
		res.machine = unitNumber
		res.unit = ""
	}

	return res, nil
}
