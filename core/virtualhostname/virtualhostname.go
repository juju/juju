// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/names/v6"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

const (
	// ErrNoMatch indicates that the hostname could not be parsed.
	ErrNoMatch = errors.ConstError("could not parse hostname")
	Domain     = "juju.local"

	hostnameLabelPattern = `[a-zA-Z0-9-]+`
	unitNumberPattern    = `\d+`
	modelUUIDPattern     = `[0-9a-fA-F-]+`
	domainPattern        = `[a-zA-Z0-9.-]+`
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
func NewInfoMachineTarget(modelUUID string, machine string) (Info, error) {
	if !names.IsValidModel(modelUUID) {
		return Info{}, errors.Errorf("invalid model UUID: %q", modelUUID)
	}
	machineName := coremachine.Name(machine)
	if err := machineName.Validate(); err != nil {
		return Info{}, errors.Errorf("invalid machine name %q: %w", machine, err)
	}
	return newInfo(MachineTarget, modelUUID, machineName, 0, "", "")
}

// NewInfoUnitTarget returns a new Info struct for a unit target.
func NewInfoUnitTarget(modelUUID string, unit string) (Info, error) {
	if !names.IsValidModel(modelUUID) {
		return Info{}, errors.Errorf("invalid model UUID: %q", modelUUID)
	}
	if !names.IsValidUnit(unit) {
		return Info{}, errors.Errorf("invalid unit name: %s", unit)
	}
	unitNumber, err := names.UnitNumber(unit)
	if err != nil {
		return Info{}, errors.Capture(err)
	}
	applicationName, err := names.UnitApplication(unit)
	if err != nil {
		return Info{}, errors.Capture(err)
	}
	return newInfo(UnitTarget, modelUUID, "", unitNumber, applicationName, "")
}

// NewInfoContainerTarget returns a new Info struct for a container target.
func NewInfoContainerTarget(modelUUID string, unit string, container string) (Info, error) {
	if !names.IsValidModel(modelUUID) {
		return Info{}, errors.Errorf("invalid model UUID: %q", modelUUID)
	}
	if !names.IsValidUnit(unit) {
		return Info{}, errors.Errorf("invalid unit name: %s", unit)
	}
	unitNumber, err := names.UnitNumber(unit)
	if err != nil {
		return Info{}, errors.Capture(err)
	}
	applicationName, err := names.UnitApplication(unit)
	if err != nil {
		return Info{}, errors.Capture(err)
	}
	return newInfo(ContainerTarget, modelUUID, "", unitNumber, applicationName, container)
}

// newInfo returns a new Info struct for the given target.
func newInfo(target HostnameTarget, modelUUID string, machine coremachine.Name, unitNumber int, applicationName string, container string) (Info, error) {
	info := Info{}
	switch target {
	case MachineTarget:
		info.target = MachineTarget
		info.modelUUID = coremodel.UUID(modelUUID)
		info.machine = machine
	case UnitTarget:
		info.target = UnitTarget
		info.modelUUID = coremodel.UUID(modelUUID)
		info.unitNumber = unitNumber
		info.applicationName = applicationName
	case ContainerTarget:
		info.target = ContainerTarget
		info.modelUUID = coremodel.UUID(modelUUID)
		info.unitNumber = unitNumber
		info.applicationName = applicationName
		info.container = container
	default:
		return Info{}, errors.Errorf("unknown target: %d", target)
	}
	return info, nil
}

// Info returns a breakdown of a virtual
// hostname into its constituent parts.
// The target field indicates what kind of
// hostname was parsed which will indicate
// that some fields are empty.
type Info struct {
	target          HostnameTarget
	modelUUID       coremodel.UUID
	machine         coremachine.Name
	applicationName string
	unitNumber      int
	container       string
}

// Unit returns the unit name.
func (i Info) Unit() (string, bool) {
	return fmt.Sprintf("%s/%d", i.applicationName, i.unitNumber), i.target != MachineTarget
}

// Container returns the container name
// and a bool to indicate if a container
// is valid for the target type.
func (i Info) Container() (string, bool) {
	return i.container, i.target == ContainerTarget
}

// ModelUUID returns the model UUID.
func (i Info) ModelUUID() coremodel.UUID {
	return i.modelUUID
}

// Machine returns the machine name.
func (i Info) Machine() (coremachine.Name, bool) {
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
		return fmt.Sprintf("%s.%s.%s", encodeMachineName(i.machine.String()), i.modelUUID, Domain)
	case UnitTarget:
		return fmt.Sprintf("%d.%s.%s.%s", i.unitNumber, i.applicationName, i.modelUUID, Domain)
	case ContainerTarget:
		return fmt.Sprintf("%s.%d.%s.%s.%s", i.container, i.unitNumber, i.applicationName, i.modelUUID, Domain)
	default:
		return "unknown"
	}
}

var (
	// machineHostnameMatcher parses a machine hostname of the following format:
	// Machine: 1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	// Nested container machine: 1-lxd-0.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	machineHostnameMatcher = regexp.MustCompile(
		`^(?<machine>` + hostnameLabelPattern + `)\.(?<modeluuid>` + modelUUIDPattern + `)\.(?<domain>` + domainPattern + `)$`,
	)

	// unitHostnameMatcher parses a unit hostname of the following format:
	// Unit: 1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	unitHostnameMatcher = regexp.MustCompile(
		`^(?<unitnumber>` + unitNumberPattern + `)\.(?<appname>` + hostnameLabelPattern + `)\.(?<modeluuid>` + modelUUIDPattern + `)\.(?<domain>` + domainPattern + `)$`,
	)

	// containerHostnameMatcher parses a unit container hostname of the following format:
	// Container: charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local
	containerHostnameMatcher = regexp.MustCompile(
		`^(?<containername>` + hostnameLabelPattern + `)\.(?<unitnumber>` + unitNumberPattern + `)\.(?<appname>` + hostnameLabelPattern + `)\.(?<modeluuid>` + modelUUIDPattern + `)\.(?<domain>` + domainPattern + `)$`,
	)
)

func encodeMachineName(machineName string) string {
	return strings.ReplaceAll(machineName, "/", "-")
}

func decodeMachineName(machineLabel string) (string, error) {
	machineName := strings.ReplaceAll(machineLabel, "-", "/")
	if err := coremachine.Name(machineName).Validate(); err != nil {
		return "", errors.Errorf("invalid machine name %q: %w", machineName, err)
	}
	return machineName, nil
}

func matchSubexpressions(matcher *regexp.Regexp, hostname string) map[string]string {
	match := matcher.FindStringSubmatch(hostname)
	if match == nil {
		return nil
	}
	result := make(map[string]string)
	for i, name := range matcher.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	return result
}

// Parse parses a virtual Juju hostname
// that references entities like machines, units
// and containers.
func Parse(hostname string) (Info, error) {
	if result := matchSubexpressions(containerHostnameMatcher, hostname); result != nil {
		return parseContainerHostname(result)
	}
	if result := matchSubexpressions(unitHostnameMatcher, hostname); result != nil {
		return parseUnitHostname(result)
	}
	if result := matchSubexpressions(machineHostnameMatcher, hostname); result != nil {
		return parseMachineHostname(result)
	}
	return Info{}, ErrNoMatch
}

func parseContainerHostname(result map[string]string) (Info, error) {
	if !names.IsValidModel(result["modeluuid"]) {
		return Info{}, errors.Errorf("invalid model UUID: %q", result["modeluuid"])
	}
	if !names.IsValidApplication(result["appname"]) {
		return Info{}, errors.Errorf("invalid application name: %q", result["appname"])
	}
	unitNumber, err := strconv.Atoi(result["unitnumber"])
	if err != nil {
		return Info{}, errors.Errorf("failed to parse unit number: %w", err)
	}

	return Info{
		target:          ContainerTarget,
		modelUUID:       coremodel.UUID(result["modeluuid"]),
		container:       result["containername"],
		unitNumber:      unitNumber,
		applicationName: result["appname"],
	}, nil
}

func parseUnitHostname(result map[string]string) (Info, error) {
	if !names.IsValidModel(result["modeluuid"]) {
		return Info{}, errors.Errorf("invalid model UUID: %q", result["modeluuid"])
	}
	if !names.IsValidApplication(result["appname"]) {
		return Info{}, errors.Errorf("invalid application name: %q", result["appname"])
	}
	unitNumber, err := strconv.Atoi(result["unitnumber"])
	if err != nil {
		return Info{}, errors.Errorf("failed to parse unit number: %w", err)
	}

	return Info{
		target:          UnitTarget,
		modelUUID:       coremodel.UUID(result["modeluuid"]),
		unitNumber:      unitNumber,
		applicationName: result["appname"],
	}, nil
}

func parseMachineHostname(result map[string]string) (Info, error) {
	if !names.IsValidModel(result["modeluuid"]) {
		return Info{}, errors.Errorf("invalid model UUID: %q", result["modeluuid"])
	}
	machineName, err := decodeMachineName(result["machine"])
	if err != nil {
		return Info{}, err
	}

	return Info{
		target:    MachineTarget,
		modelUUID: coremodel.UUID(result["modeluuid"]),
		machine:   coremachine.Name(machineName),
	}, nil
}
