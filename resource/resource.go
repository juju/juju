// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"fmt"
	"time"

	"github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
)

// Resource defines a single resource within a Juju model.
//
// Each application will have have exactly the same resources associated
// with it as are defined in the charm's metadata, no more, no less.
// When associated with the application the resource may have additional
// information associated with it.
//
// A resource may be a "placeholder", meaning it is only partially
// populated before an upload (whether local or from the charm store).
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
//
// A resource may also be added to the model as "pending", meaning it
// is queued up to be used as a resource for the application. Until it is
// "activated", a pending resources is virtually invisible. There may
// be more that one pending resource for a given resource ID.
type Resource struct {
	resource.Resource

	// ID uniquely identifies a resource-application pair within the model.
	// Note that the model ignores pending resources (those with a
	// pending ID) except for in a few clearly pending-related places.
	// ID may be empty if the ID (assigned by the model) is not known.
	ID string

	// PendingID identifies that this resource is pending and
	// distinguishes it from other pending resources with the same model
	// ID (and from the active resource). The active resource for the
	// applications will not have PendingID set.
	PendingID string

	// TODO(ericsnow) Use names.ApplicationTag for applicationID?

	// ApplicationID identifies the application for the resource.
	ApplicationID string

	// TODO(ericsnow) Use names.UserTag for Username?

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

	if res.ApplicationID == "" {
		return errors.NewNotValid(nil, "missing application ID")
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
	switch res.Origin {
	case resource.OriginUpload:
		if res.IsPlaceholder() {
			return "-"
		}
		return res.TimestampGranular().UTC().String()
	case resource.OriginStore:
		return fmt.Sprintf("%d", res.Revision)
	default:
		// note: this should probably never happen.
		return "-"
	}
}

// AsMap returns the mapping of resource name to info for each of the
// given resources.
func AsMap(resources []Resource) map[string]Resource {
	results := make(map[string]Resource, len(resources))
	for _, res := range resources {
		results[res.Name] = res
	}
	return results
}
