// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

// TODO(ericsnow) Move the this file or something similar to the charm repo?

// NoRevision indicates that the spec does not have a revision specified.
const NoRevision = ""

// Spec describes one resource that a service uses.
type Spec struct {
	// Definition is the basic info about the resource.
	Definition resource.Info

	// Origin identifies where the resource should come from.
	Origin Origin

	// Revision is the desired revision of the resource. It returns ""
	// for origins that do not support revisions.
	Revision string
}

// Validate ensures that the spec is valid.
func (s Spec) Validate() error {
	if err := s.Definition.Validate(); err != nil {
		return errors.NewNotValid(err, "")
	}

	switch s.Origin {
	case OriginUpload:
		if s.Revision != "" {
			return errors.NewNotValid(nil, `"upload" specs don't have revisions`)
		}
		return nil
	default:
		return errors.NewNotValid(nil, fmt.Sprintf("resource origin %q", s.Origin))
	}
}
