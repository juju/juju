// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"

	"github.com/juju/errors"
)

// Resource describes a charm's resource in the charm store.
type Resource struct {
	Meta

	// Origin identifies where the resource will come from.
	Origin Origin

	// Revision is the charm store revision of the resource.
	Revision int

	// Fingerprint is the SHA-384 checksum for the resource blob.
	Fingerprint Fingerprint

	// Size is the size of the resource, in bytes.
	Size int64
}

// Validate checks the payload class to ensure its data is valid.
func (res Resource) Validate() error {
	if err := res.Meta.Validate(); err != nil {
		return errors.Annotate(err, "bad metadata")
	}

	if err := res.Origin.Validate(); err != nil {
		return errors.Annotate(err, "bad origin")
	}

	if err := res.validateRevision(); err != nil {
		return errors.Annotate(err, "bad revision")
	}

	if res.Type == TypeFile {
		if err := res.validateFileInfo(); err != nil {
			return errors.Annotate(err, "bad file info")
		}
	}

	return nil
}

func (res Resource) validateRevision() error {
	if res.Origin == OriginUpload {
		// We do not care about the revision, so we don't check it.
		// TODO(ericsnow) Ensure Revision is 0 for OriginUpload?
		return nil
	}

	if res.Revision < 0 && res.isFileAvailable() {
		return errors.NewNotValid(nil, fmt.Sprintf("must be non-negative, got %d", res.Revision))
	}

	return nil
}

func (res Resource) validateFileInfo() error {
	if res.Fingerprint.IsZero() {
		if res.Size > 0 {
			return errors.NewNotValid(nil, "missing fingerprint")
		}
	} else {
		if err := res.Fingerprint.Validate(); err != nil {
			return errors.Annotate(err, "bad fingerprint")
		}
	}

	if res.Size < 0 {
		return errors.NewNotValid(nil, "negative size")
	}

	return nil
}

// isFileAvailable determines whether or not the resource info indicates
// that the resource file is available.
func (res Resource) isFileAvailable() bool {
	if !res.Fingerprint.IsZero() {
		return true
	}
	if res.Size > 0 {
		return true
	}
	return false
}
