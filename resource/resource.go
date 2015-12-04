// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"fmt"

	"github.com/juju/errors"
)

var originRevisionTypes = map[OriginKind]RevisionType{
	OriginKindUpload: RevisionTypeDate,
}

// Resource defines a single resource within Juju state.
type Resource struct {
	Spec Spec

	// Origin identifies the where the resource came from.
	Origin Origin

	// Revision is the actual revision of the resource.
	Revision Revision

	// Fingerprint is the MD5 checksum for the resource blob.
	Fingerprint string
}

// Validate ensures that the spec is valid.
func (res Resource) Validate() error {
	if err := res.Spec.Validate(); err != nil {
		return errors.Annotate(err, "bad spec")
	}

	if err := res.Origin.Validate(); err != nil {
		return errors.Annotate(err, "bad origin")
	}
	if res.Origin.Kind != res.Spec.Origin {
		return errors.NotValidf("origin kind does not match spec (expected %q)", res.Spec.Origin)
	}

	if err := res.Revision.Validate(); err != nil {
		return errors.Annotate(err, "bad revision")
	}

	revType := res.Revision.Type
	if originRevisionTypes[res.Origin.Kind] != revType {
		return errors.NewNotValid(nil, fmt.Sprintf("resource origin %q does not support revision type %q", res.Origin, revType))
	}

	if res.Fingerprint == "" {
		return errors.NewNotValid(nil, "missing fingerprint")
	}

	return nil
}
