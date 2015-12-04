// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"

	"github.com/juju/errors"
)

// TODO(ericsnow) This file probably belongs in the charm repo.

// This revision machinery is necessary because there is more than one
// kind of revision: integers for charm store resources and dates for
// uploaded resources. Additionally, specs for uploaded resources do not
// have any revision.

// TODO(ericsnow) Type -> Kind.

// These are the recognized revision types (except for unknown).
const (
	RevisionTypeUnknown RevisionType = ""
	RevisionTypeNone    RevisionType = "no revision"
)

var revisionTypes = map[RevisionType]func(string) bool{
	RevisionTypeNone: func(s string) bool { return len(s) == 0 },
}

// RevisionType identifies a type of resource revision (e.g. date, int).
type RevisionType string

// Validate ensures that the revision type is correct.
func (rt RevisionType) Validate() error {
	if _, ok := revisionTypes[rt]; !ok {
		return errors.NewNotValid(nil, "unknown revision type")
	}
	return nil
}

// NoRevision indicates that the spec does not have a revision specified.
var NoRevision Revision = Revision{
	Type:  RevisionTypeNone,
	Value: "",
}

// Revision identifies a resouce revision.
type Revision struct {
	Type RevisionType

	Value string
}

// ParseRevision converts the provided value into a Revision. If it
// cannot be converted then false is returned.
func ParseRevision(value string) (Revision, error) {
	rev := Revision{
		Value: value,
	}

	for rt, match := range revisionTypes {
		if match(value) {
			rev.Type = rt
			return rev, nil
		}
	}

	if err := rev.Validate(); err != nil {
		return rev, errors.Trace(err)
	}
	return rev, nil
}

// String returns the printable representation of the revision.
func (rev Revision) String() string {
	return rev.Value
}

// Validate ensures that the revision is correct.
func (rev Revision) Validate() error {
	if err := rev.Type.Validate(); err != nil {
		return errors.Annotate(err, "bad revision type")
	}

	match := revisionTypes[rev.Type]
	if !match(rev.Value) {
		msg := fmt.Sprintf("invalid value %q for revision type %s", rev.Value, rev.Type)
		return errors.NewNotValid(nil, msg)
	}

	return nil
}
