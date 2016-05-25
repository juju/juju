// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
)

// Hostname returns a name suitable to be used for a machine hostname.
// This function returns an error if either the model or machine tags are invalid.
func Hostname(model names.ModelTag, machine names.MachineTag) (string, error) {
	uuid := model.Id()
	// TODO: would be nice if the tags exported a method Valid().
	if !names.IsValidModel(uuid) {
		return "", errors.Errorf("model ID %q is not a valid model", uuid)
	}
	// The suffix is the last six hex digits of the model uuid.
	suffix := uuid[len(uuid)-6:]

	machineID := machine.Id()
	if !names.IsValidMachine(machineID) {
		return "", errors.Errorf("machine ID %q is not a valid machine", machineID)
	}
	machineID = strings.Replace(machineID, "/", "-", -1)

	return "juju-" + suffix + "-" + machineID, nil
}

// Namespace provides a way to generate machine hostanmes with a given prefix.
type Namespace interface {
	// Prefix returns the common part of the hostnames. i.e. 'juju-xxxxxx-'
	Prefix() string
	// Hostname returns a name suitable to be used for a machine hostname.
	// This function returns an error if the machine tags is invalid.
	Hostname(machine names.MachineTag) (string, error)

	// MachineTag does the reverse of the Hostname method, and extracts the
	// Tag from the hostname.
	MachineTag(hostname string) (names.MachineTag, error)
}

type namespace struct {
	name string
}

// NewNamespace returns a Namespace identified by the last six hex digits of the
// model UUID. NewNamespace returns an error if the model tag is invalid.
func NewNamespace(model names.ModelTag) (Namespace, error) {
	uuid := model.Id()
	// TODO: would be nice if the tags exported a method Valid().
	if !names.IsValidModel(uuid) {
		return nil, errors.Errorf("model ID %q is not a valid model", uuid)
	}
	// The suffix is the last six hex digits of the model uuid.
	suffix := uuid[len(uuid)-6:]

	return &namespace{name: suffix}, nil
}

// Hostname implements Namespace.
func (n *namespace) Hostname(machine names.MachineTag) (string, error) {
	machineID := machine.Id()
	if !names.IsValidMachine(machineID) {
		return "", errors.Errorf("machine ID %q is not a valid machine", machineID)
	}
	machineID = strings.Replace(machineID, "/", "-", -1)

	return n.Prefix() + machineID, nil
}

// Hostname implements Namespace.
func (n *namespace) MachineTag(hostname string) (names.MachineTag, error) {
	prefix := n.Prefix()
	if !strings.HasPrefix(hostname, prefix) {
		return names.MachineTag{}, errors.Errorf("hostname %q not from namespace %q", hostname, prefix)
	}
	id := hostname[len(prefix):]
	id = strings.Replace(id, "-", "/", -1)
	if !names.IsValidMachine(id) {
		return names.MachineTag{}, errors.Errorf("unexpected machine id %q", id)
	}
	return names.NewMachineTag(id), nil
}

// Prefix implements Namespace.
func (n *namespace) Prefix() string {
	return "juju-" + n.name + "-"
}
