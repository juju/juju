// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// Resource defines a single resource within a Juju model.
//
// Each application will have exactly the same resources associated
// with it as are defined in the charm's metadata, no more, no less.
// When associated with the application the resource may have additional
// information associated with it.
//
// A resource may be a "placeholder", meaning it is only partially
// populated before an upload (whether local or from the charm store).
// In that case the following fields are not set:
//
//	UUID, Timestamp, RetrievedBy
//
// For "upload" placeholders, the following additional fields are
// not set:
//
//	Fingerprint, Size
type Resource struct {
	resource.Resource

	UUID UUID

	// ApplicationName identifies the application name for the resource
	ApplicationName string

	// RetrievedBy is the name of who added the resource to the controller.
	// The name is a username if the resource is uploaded from the cli
	// by a specific user. If the resource is downloaded from a repository,
	// the ID of the unit which triggered the download is used.
	RetrievedBy string

	// Timestamp indicates when this resource was added to the model in
	// the case of applications or when this resource was loaded by a unit.
	Timestamp time.Time
}

// RetrievedByType indicates what the RetrievedBy name represents.
type RetrievedByType string

const (
	Unknown     RetrievedByType = "unknown"
	Application RetrievedByType = "application"
	Unit        RetrievedByType = "unit"
	User        RetrievedByType = "user"
)

func (r RetrievedByType) String() string {
	return string(r)
}

// Validate ensures that the spec is valid.
func (res Resource) Validate() error {
	// TODO(ericsnow) Ensure that the "placeholder" fields are not set
	// if IsLocalPlaceholder() returns true (and that they *are* set
	// otherwise)? Also ensure an "upload" origin in the "placeholder"
	// case?

	if err := res.Resource.Validate(); err != nil {
		return errors.Errorf("bad info: %w", err)
	}

	if res.ApplicationName == "" {
		return errors.Errorf("missing application name: %w", coreerrors.NotValid)
	}

	// TODO(ericsnow) Require that RetrievedBy be set if timestamp is?
	if res.Timestamp.IsZero() && res.RetrievedBy != "" {
		return errors.New("missing timestamp").Add(coreerrors.NotValid)
	}

	return nil
}

// IsPlaceholder indicates if the resource is a
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
