// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
)

// These are the valid kinds of resource origin.
const (
	originKindUnknown OriginKind = iota
	OriginKindUpload
	OriginKindStore
)

var knownOriginKinds = map[OriginKind]string{
	OriginKindUpload: "upload",
	OriginKindStore:  "store",
}

// OriginKind identifies the kind of a resource origin.
type OriginKind int

// ParseOriginKind converts the provided string into an OriginKind.
// If it is not a known origin kind then an error is returned.
func ParseOriginKind(value string) (OriginKind, error) {
	for kind, str := range knownOriginKinds {
		if value == str {
			return kind, nil
		}
	}
	return originKindUnknown, errors.Errorf("unknown origin %q", value)
}

// String returns the printable representation of the origin kind.
func (o OriginKind) String() string {
	return knownOriginKinds[o]
}

// Validate ensures that the origin is correct.
func (o OriginKind) Validate() error {
	// Ideally, only the (unavoidable) zero value would be invalid.
	// However, typedef'ing int means that the use of int literals
	// could result in invalid Type values other than the zero value.
	if _, ok := knownOriginKinds[o]; !ok {
		return errors.NewNotValid(nil, "unknown origin")
	}
	return nil
}
