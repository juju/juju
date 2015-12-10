// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
)

// TODO(ericsnow) This file probably belongs in the charm repo.

// This revision machinery is necessary because there is more than one
// kind of revision: integers for charm store resources and dates for
// uploaded resources. Additionally, specs for uploaded resources do not
// have any revision.

// These are the recognized revision types (except for unknown).
const (
	RevisionTypeUnknown RevisionType = ""
	RevisionTypeNone                 = "no revision"
)

// RevisionType identifies a type of resource revision (e.g. date, int).
type RevisionType string

// NoRevision indicates that the spec does not have a revision specified.
const NoRevision Revision = ""

// Revision identifies a resouce revision.
type Revision string

// ParseRevision converts the provided value into a Revision. If it
// cannot be converted then an error is returned.
func ParseRevision(value string) (Revision, error) {
	if value == "" {
		return NoRevision, nil
	}
	rev := Revision(value)

	if err := rev.Validate(); err != nil {
		return rev, errors.Trace(err)
	}
	return rev, nil
}

// String returns the printable representation of the revision.
func (rev Revision) String() string {
	return string(rev)
}

// Type returns the revision's type.
func (rev Revision) Type() RevisionType {
	if rev == NoRevision {
		return RevisionTypeNone
	}

	return RevisionTypeUnknown
}

// Validate ensures that the revision is correct.
func (rev Revision) Validate() error {
	if rev.Type() == RevisionTypeUnknown {
		return errors.NewNotValid(nil, "unrecognized revision type")
	}

	return nil
}
