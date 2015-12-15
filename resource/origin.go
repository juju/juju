// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
)

// These are the valid kinds of resource origin.
var (
	OriginKindUpload = OriginKind{"upload"}
	OriginKindStore  = OriginKind{"store"}
)

var knownOriginKinds = map[OriginKind]bool{
	OriginKindUpload: true,
	OriginKindStore:  true,
}

// OriginKind identifies the kind of a resource origin.
type OriginKind struct {
	str string
}

// ParseOriginKind converts the provided string into an OriginKind.
// If it is not a known origin kind then an error is returned.
func ParseOriginKind(value string) (OriginKind, error) {
	for kind := range knownOriginKinds {
		if value == kind.str {
			return kind, nil
		}
	}
	return OriginKind{}, errors.Errorf("unknown origin %q", value)
}

// String returns the printable representation of the origin kind.
func (o OriginKind) String() string {
	return o.str
}

// Validate ensures that the origin is correct.
func (o OriginKind) Validate() error {
	// Only the zero value is invalid.
	var zero OriginKind
	if o == zero {
		return errors.NewNotValid(nil, "unknown origin")
	}
	return nil
}
