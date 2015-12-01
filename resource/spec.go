// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

// TODO(ericsnow) Move the this file or something similar to the charm repo?

// NoRevision indicates that the spec does not have a revision specified.
const NoRevision = ""

// Spec describes one resource that a service uses.
type Spec interface {
	// Definition is the basic info about the resource.
	Definition() resource.Info

	// Origin identifies where the resource should come from.
	Origin() Origin

	// Revision is the desired revision of the resource. It returns ""
	// for origins that do not support revisions.
	Revision() string
}

// NewSpec returns a new Spec for the given info.
func NewSpec(info resource.Info, origin Origin, revision string) (Spec, error) {
	switch origin {
	case OriginUpload:
		// TODO(ericsnow) Fail if revision not NoRevision?
		return &uploadSpec{info}, nil
	default:
		return nil, errors.NotSupportedf("resource origin %q", origin)
	}
}

// uploadSpec defines an *uploaded* resource that a service expects.
type uploadSpec struct {
	resource.Info
}

// Definition implements Spec.
func (res uploadSpec) Definition() resource.Info {
	return res.Info
}

// Origin implements Spec.
func (res uploadSpec) Origin() Origin {
	return OriginUpload
}

// Revision implements Spec.
func (res uploadSpec) Revision() string {
	return NoRevision
}
