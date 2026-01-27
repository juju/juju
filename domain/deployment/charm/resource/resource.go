// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	coreerrors "github.com/juju/juju/core/errors"
	internalerrors "github.com/juju/juju/internal/errors"
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
		return internalerrors.Errorf("bad metadata: %w", err)
	}

	if err := res.Origin.Validate(); err != nil {
		return internalerrors.Errorf("bad origin: %w", err)
	}

	if err := res.validateRevision(); err != nil {
		return internalerrors.Errorf("bad revision: %w", err)
	}

	if res.Type == TypeFile {
		if err := res.validateFileInfo(); err != nil {
			return internalerrors.Errorf("bad file info: %w", err)
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
		return internalerrors.Errorf("must be non-negative, got %d", res.Revision).Add(coreerrors.NotValid)
	}

	return nil
}

func (res Resource) validateFileInfo() error {
	if res.Fingerprint.IsZero() {
		if res.Size > 0 {
			return internalerrors.Errorf("missing fingerprint").Add(coreerrors.NotValid)
		}
	} else {
		if err := res.Fingerprint.Validate(); err != nil {
			return internalerrors.Errorf("bad fingerprint: %w", err)
		}
	}

	if res.Size < 0 {
		return internalerrors.Errorf("negative size").Add(coreerrors.NotValid)
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
