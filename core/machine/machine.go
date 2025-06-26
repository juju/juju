// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"regexp"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

const (
	// numberPattern is a non-compiled regexp that can be composed with other
	// snippets for validating small number sequences.
	//
	// Numbers are a series of digits, with no leading zeros unless the number
	// is exactly 0.
	numberPattern = "(?:0|[1-9][0-9]*)"

	// containerTypePattern is a non-compiled regexp that matches
	// container types. It is a series of lower case letters (lxd for example).
	containerTypePattern = "[a-z]+"

	// containerPattern is a non-compiled regexp that matches
	// a container name. It is a slash followed by a container type and
	// a number, for example: /lxd/0.
	containerPattern = "/" + containerTypePattern + "/" + numberPattern + ""

	// machinePattern is a non-compiled regexp that matches
	// a machine name. It is a number, followed by an optional
	// series of containers. Each container is a slash followed by a
	// container type and a number, for example: 0 or 0/lxd/0.
	machinePattern = numberPattern + "(?:" + containerPattern + ")*"
)

var validMachine = regexp.MustCompile("^" + machinePattern + "$")

// Name is a unique name identifier for a machine.
type Name string

// Validate returns an error if the [Name] is invalid. The error returned
// satisfies [errors.NotValid].
func (n Name) Validate() error {
	if !validMachine.MatchString(n.String()) {
		return errors.Errorf("machine name").Add(coreerrors.NotValid)
	}
	if strings.Count(n.String(), "/") > 2 {
		return errors.Errorf("machine name %q has too many containers", n).Add(coreerrors.NotValid)
	}
	return nil
}

// String returns the [Name] as a string.
func (i Name) String() string {
	return string(i)
}

// NamedChild returns a new [Name] that is a child of the
// current [Name]. The child name is appended to the
// current [Name] with a "/" separator. The scope is
// prepended to the child name with a "/" separator.
func (i Name) NamedChild(scope string, childName string) (Name, error) {
	if scope == "" {
		return "", errors.Errorf("empty scope").Add(coreerrors.NotValid)
	} else if childName == "" {
		return "", errors.Errorf("empty child name").Add(coreerrors.NotValid)
	}

	return Name(string(i) + "/" + scope + "/" + childName), nil
}

// IsContainer returns true if the [Name] is a container.
func (i Name) IsContainer() bool {
	// A machine name is a container if it contains a slash.
	return strings.Contains(string(i), "/")
}

// Parent returns the name of the parent machine. It returns the [Name] itself
// if the [Name] is not a child parent relationship. Otherwise it returns
// the first part of the [Name] before the first slash.
// It expects that the [Name] is a valid machine name, and does not
// perform any validation on the [Name].
func (i Name) Parent() Name {
	// A machine name is a parent if it does not contain a slash.
	if !i.IsContainer() {
		return i
	}

	// If the name is a container, we need to remove the last container from the name.
	// We do this by finding the last slash and returning everything before it.
	slash := strings.Index(string(i), "/")
	if slash < 0 {
		// If there is no slash, then the name is not a container.
		return i
	}
	return i[:slash]
}

// Child returns the name of the child machine. It returns the [Name] itself
// if the [Name] is not a child parent relationship. Otherwise it returns
// the part of the [Name] after the last slash.
func (i Name) Child() Name {
	// A machine name is a child if it contains a slash.
	if !i.IsContainer() {
		return i
	}

	// If the name is a container, we need to remove the first part of the name.
	// We do this by finding the first slash and returning everything after it.
	slash := strings.LastIndex(string(i), "/")
	if slash < 0 {
		// If there is no slash, then the name is not a container.
		return i
	}
	return i[slash+1:]
}

// RebootAction defines the action a machine should
// take when a hook needs to reboot
type RebootAction string

const (
	// ShouldDoNothing instructs a machine agent that no action
	// is required on its part
	ShouldDoNothing RebootAction = "noop"
	// ShouldReboot instructs a machine to reboot
	// this happens when a hook running on a machine, requests
	// a reboot
	ShouldReboot RebootAction = "reboot"
	// ShouldShutdown instructs a machine to shut down. This usually
	// happens when running inside a container, and a hook on the parent
	// machine requests a reboot
	ShouldShutdown RebootAction = "shutdown"
)
