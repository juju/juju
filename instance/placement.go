// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"fmt"
	"strings"

	"launchpad.net/juju-core/names"
)

const (
	// MachineScope is a special scope name that is used
	// for machine placement directives (e.g. --to 0).
	MachineScope = "#"
)

// Placement defines a placement directive, which has a scope
// and a value that is scope-specific.
type Placement struct {
	// Scope is the scope of the placement directive. Scope may
	// be a container type (lxc, kvm), instance.MachineScope, or
	// an environment name.
	//
	// If Scope is empty, then it must be inferred from the context.
	Scope string

	// Value is a scope-specific placement value.
	//
	// For MachineScope or a container scope, this may be empty or
	// the ID of an existing machine.
	Value string
}

func (p *Placement) String() string {
	return fmt.Sprintf("%s:%s", p.Scope, p.Value)
}

func isContainerType(s string) bool {
	_, err := ParseContainerType(s)
	return err == nil
}

// ParsePlacement attempts to parse the specified string and create a
// corresponding Placement structure.
func ParsePlacement(directive string) (*Placement, error) {
	if directive == "" {
		return nil, nil
	}
	if colon := strings.IndexRune(directive, ':'); colon != -1 {
		scope, value := directive[:colon], directive[colon+1:]
		// Sanity check: machine/container scopes require a machine ID as the value.
		if (scope == MachineScope || isContainerType(scope)) && !names.IsMachine(value) {
			return nil, fmt.Errorf("invalid value %q for %q scope: expected machine-id", value, scope)
		}
		return &Placement{Scope: scope, Value: value}, nil
	}
	if names.IsMachine(directive) {
		return &Placement{Scope: MachineScope, Value: directive}, nil
	}
	if isContainerType(directive) {
		return &Placement{Scope: directive}, nil
	}
	// Empty scope, caller must infer the scope from context.
	return &Placement{Value: directive}, nil
}

// ParsePlacement attempts to parse the specified string and create a
// corresponding Placement structure, panicking if an error occurs.
func MustParsePlacement(directive string) *Placement {
    placement, err := ParsePlacement(directive)
    if err != nil {
        panic(err)
    }
    return placement
}
