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

// ResourceSpec describes one resource that a service uses.
type ResourceSpec interface {
	// Definition is the basic info about the resource.
	Definition() charm.ResourceInfo

	// Origin identifies where the resource should come from.
	Origin() string

	// Revision is the desired revision of the resource. It returns ""
	// for origins that do not support revisions.
	Revision() string
}

// NewResourceSpec returns a new ResourceSpec for the given info.
func NewResourceSpec(info charm.ResourceInfo, origin, revision string) (ResourceSpec, error) {
	switch origin {
	case OriginUpload:
		// TODO(ericsnow) Fail if revision not NoRevision?
		return &UploadResourceSpec{info}, nil
	default:
		return nil, errors.NotSupportedf("resource origin %q", origin)
	}
}

// UploadResourceSpec defines an *uploaded* resource that a service expects.
type UploadResourceSpec struct {
	charm.ResourceInfo
}

// Definition implements ResourceSpec.
func (res UploadResourceSpec) Definition() charm.ResourceInfo {
	return res.ResourceInfo
}

// Origin implements ResourceSpec.
func (res UploadResourceSpec) Origin() string {
	return OriginUpload
}

// Revision implements ResourceSpec.
func (res UploadResourceSpec) Revision() string {
	return NoRevision
}
