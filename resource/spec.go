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

var revisionTypes = map[Origin]RevisionType{
	OriginUpload: RevisionTypeNone,
}

// Spec describes one resource that a service uses.
//
// Note that a spec relates to a resource as defined by a charm and not
// as realized for a service in state. There is a separate Resource type
// for that.
type Spec struct {
	// Definition is the basic info about the resource.
	Definition resource.Info

	// Origin identifies where the resource should come from.
	Origin Origin

	// Revision is the desired revision of the resource. It returns ""
	// for origins that do not support revisions.
	Revision Revision
}

// Validate ensures that the spec is valid.
func (s Spec) Validate() error {
	if err := s.Definition.Validate(); err != nil {
		return errors.NewNotValid(err, "")
	}

	switch s.Origin {
	case OriginUpload:
		if s.Revision != NoRevision {
			return errors.NewNotValid(nil, `"upload" specs don't have revisions`)
		}
		return nil
	default:
		return errors.NewNotValid(nil, fmt.Sprintf("resource origin %q not supported", s.Origin))
	}

	revType := s.Revision.Type()
	if revisionTypes[s.Origin] != revType {
		return errors.NewNotValid(nil, fmt.Sprintf("resource origin %q does not support revision type %q", s.Origin, revType))
	}

	if err := s.Revision.Validate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
