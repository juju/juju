// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// Name is a unique name identifier for a machine.
type Name string

// Validate returns an error if the [Name] is invalid. The error returned
// satisfies [errors.NotValid].
func (n Name) Validate() error {
	if n == "" {
		return errors.Errorf("empty machine name").Add(coreerrors.NotValid)
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
