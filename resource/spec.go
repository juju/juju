// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
)

// TODO(ericsnow) Move the this file or something similar to the charm repo?

// These are the valid resource origins.
const (
	OriginUpload = "upload"
)

// NoRevision indicates that the spec does not have a revision specified.
const NoRevision = ""

// Spec describes one resource that a service uses.
type Spec interface {
	// Definition is the basic info about the resource.
	Definition() charm.ResourceInfo

	// Origin identifies where the resource should come from.
	Origin() string

	// Revision is the desired revision of the resource. It returns ""
	// for origins that do not support revisions.
	Revision() string
}

// NewSpec returns a new Spec for the given info.
func NewSpec(info charm.ResourceInfo, origin, revision string) (Spec, error) {
	switch origin {
	case OriginUpload:
		// TODO(ericsnow) Fail if revision not NoRevision?
		return &UploadSpec{info}, nil
	default:
		return nil, errors.NotSupportedf("resource origin %q", origin)
	}
}

// UploadSpec defines an *uploaded* resource that a service expects.
type UploadSpec struct {
	charm.ResourceInfo
}

// Definition implements Spec.
func (res UploadSpec) Definition() charm.ResourceInfo {
	return res.ResourceInfo
}

// Origin implements Spec.
func (res UploadSpec) Origin() string {
	return OriginUpload
}

// Revision implements Spec.
func (res UploadSpec) Revision() string {
	return NoRevision
}
