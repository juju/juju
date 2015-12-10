// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

// Info holds the information provided by the charm store
// about a resource.
type Info struct {
	resource.Info

	// TODO(ericsnow) Should Origin use an "Origin" type that has a Kind field?

	// Origin identifies the where the resource came from.
	Origin OriginKind

	// Revision is the revision of the resource. Uploaded resouces
	// do not have a revision.
	Revision int

	// TODO(ericsnow) Move Resource.Fingerprint here?
}

// Validate ensures that the spec is valid.
func (info Info) Validate() error {
	if err := info.Info.Validate(); err != nil {
		return errors.NewNotValid(err, "")
	}

	if err := info.Origin.Validate(); err != nil {
		return errors.Annotate(err, "bad origin")
	}

	// TODO(ericsnow) Check info.Revision?

	return nil
}
