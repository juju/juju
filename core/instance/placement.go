// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"fmt"
	"strings"

	"github.com/juju/names/v6"

	"github.com/juju/juju/internal/errors"
)

const (
	// MachineScope is a special scope name that is used
	// for machine placement directives (e.g. --to 0).
	MachineScope = "#"
)

var ErrPlacementScopeMissing = errors.Errorf("placement scope missing")

// Placement defines a placement directive, which has a scope
// and a value that is scope-specific.
type Placement struct {
	// Scope is the scope of the placement directive. Scope may
	// be a container type (lxd, kvm), instance.MachineScope, or
	// an environment name.
	//
	// If Scope is empty, then it must be inferred from the context.
	Scope string `json:"scope"`

	// Directive is a scope-specific placement directive.
	//
	// For MachineScope or a container scope, this may be empty or
	// the ID of an existing machine.
	Directive string `json:"directive"`
}

func (p *Placement) String() string {
	return fmt.Sprintf("%s:%s", p.Scope, p.Directive)
}

func isContainerType(s string) bool {
	_, err := ParseContainerType(s)
	return err == nil
}

// ParsePlacement attempts to parse the specified string and create a
// corresponding Placement structure.
//
// If the placement directive is non-empty and missing a scope,
// ErrPlacementScopeMissing will be returned as well as a Placement
// with an empty Scope field.
func ParsePlacement(directive string) (*Placement, error) {
	if directive == "" {
		return nil, nil
	}
	if colon := strings.IndexRune(directive, ':'); colon != -1 {
		scope, directive := directive[:colon], directive[colon+1:]
		if scope == "" {
			return nil, ErrPlacementScopeMissing
		}
		// Sanity check: machine/container scopes require a machine ID as the value.
		if (scope == MachineScope || isContainerType(scope)) && !names.IsValidMachine(directive) {
			return nil, errors.Errorf("invalid value %q for %q scope: expected machine-id", directive, scope)
		}
		return &Placement{Scope: scope, Directive: directive}, nil
	}
	if names.IsValidMachine(directive) {
		return &Placement{Scope: MachineScope, Directive: directive}, nil
	}
	if isContainerType(directive) {
		return &Placement{Scope: directive}, nil
	}
	return nil, ErrPlacementScopeMissing
}

// MustParsePlacement attempts to parse the specified string and create
// a corresponding Placement structure, panicking if an error occurs.
func MustParsePlacement(directive string) *Placement {
	placement, err := ParsePlacement(directive)
	if err != nil {
		panic(err)
	}
	return placement
}
