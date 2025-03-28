// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"strings"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// uuidSuffixDigits defines how many of the uuid digits to use.
// Since the NewNamespace function asserts that the modelUUID is valid, we know
// it follows the UUID string format that ends with eight hex digits.
const uuidSuffixDigits = 6

// Namespace provides a way to generate machine hostanmes with a given prefix.
type Namespace interface {
	// Prefix returns the common part of the hostnames. i.e. 'juju-xxxxxx-'
	Prefix() string

	// Hostname returns a name suitable to be used for a machine hostname.
	// This function returns an error if the machine tags is invalid.
	Hostname(machineID string) (string, error)

	// MachineTag does the reverse of the Hostname method, and extracts the
	// Tag from the hostname.
	MachineTag(hostname string) (names.MachineTag, error)

	// Value returns the input prefixed with the namespace prefix.
	Value(string) string
}

type namespace struct {
	name string
}

// NewNamespace returns a Namespace identified by the last six hex digits of the
// model UUID. NewNamespace returns an error if the model tag is invalid.
func NewNamespace(modelUUID string) (Namespace, error) {
	if !names.IsValidModel(modelUUID) {
		return nil, errors.Errorf("model UUID %q %w", modelUUID, coreerrors.NotValid)
	}
	// The suffix is the last six hex digits of the model uuid.
	suffix := modelUUID[len(modelUUID)-uuidSuffixDigits:]
	return &namespace{name: suffix}, nil
}

// Hostname implements Namespace.
func (n *namespace) Hostname(machineID string) (string, error) {
	if !names.IsValidMachine(machineID) {
		return "", errors.Errorf("machine ID %q is not a valid machine", machineID)
	}
	machineID = strings.Replace(machineID, "/", "-", -1)
	return n.Value(machineID), nil
}

// Value returns the input prefixed with the namespace prefix.
func (n *namespace) Value(s string) string {
	return n.Prefix() + s
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
