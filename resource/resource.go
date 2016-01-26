// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

// Resource defines a single resource within Juju state.
//
// A resource may be a "placeholder", meaning it is only partially
// populated pending an upload (whether local or from the charm store).
// In that case the following fields are not set:
//
//   Timestamp
//   Username
//
// For "upload" placeholders, the following additional fields are
// not set:
//
//   Fingerprint
//   Size
type Resource struct {
	resource.Resource

	// Username is the ID of the user that added the revision
	// to the model (whether implicitly or explicitly).
	Username string

	// Timestamp indicates when the resource was added to the model.
	Timestamp time.Time
}

// Validate ensures that the spec is valid.
func (res Resource) Validate() error {
	// TODO(ericsnow) Ensure that the "placeholder" fields are not set
	// if IsLocalPlaceholder() returns true (and that they *are* set
	// otherwise)? Also ensure an "upload" origin in the "placeholder"
	// case?

	if err := res.Resource.Validate(); err != nil {
		return errors.Annotate(err, "bad info")
	}

	// TODO(ericsnow) Require that Username be set if timestamp is?

	if res.Timestamp.IsZero() && res.Username != "" {
		return errors.NewNotValid(nil, "missing timestamp")
	}

	return nil
}

// IsPlaceholder indicates whether or not the resource is a
// "placeholder" (partially populated pending an upload).
func (res Resource) IsPlaceholder() bool {
	return res.Timestamp.IsZero()
}

// TimestampGranular returns the timestamp at a resolution of 1 second.
func (res Resource) TimestampGranular() time.Time {
	return time.Unix(res.Timestamp.Unix(), 0)
}

// RevisionString returns the human-readable revision for the resource.
func (res Resource) RevisionString() string {
	if res.Origin == resource.OriginUpload {
		return res.TimestampGranular().String()
	}
	return fmt.Sprintf("%d", res.Revision)
}

// Unit represents a Juju unit.
type Unit interface {
	// Name is the name of the Unit.
	Name() string

	// ServiceName is the name of the service to which the unit belongs.
	ServiceName() string
}

// ServiceResources contains the list of resources for the service and all its
// units.
type ServiceResources struct {
	// Resources are the current version of the resource for the service that
	// resource-get will retrieve.
	Resources []Resource

	// UnitResources reports the currenly-in-use version of resources for each
	// unit.
	UnitResources []UnitResources
}

// UnitResources conains the list of resources used by a unit.
type UnitResources struct {
	// Tag is the tag of the unit.
	Tag names.UnitTag

	// Resources are the resource versions currently in use by this unit.
	Resources []Resource
}
