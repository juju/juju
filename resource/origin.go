// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
)

// These are the valid kinds of resource origin.
const (
	OriginKindUnknown OriginKind = ""
	OriginKindUpload  OriginKind = "upload"
	OriginKindStore   OriginKind = "store"
)

var knownOriginKinds = map[OriginKind]bool{
	OriginKindUpload: true,
	OriginKindStore:  true,
}

// OriginKind identifies the kind of a resource origin.
type OriginKind string

// ParseOriginKind converts the provided string into an OriginKind.
// If it is not a known origin kind then false is returned.
func ParseOriginKind(value string) (OriginKind, bool) {
	o := OriginKind(value)
	return o, knownOriginKinds[o]
}

// String returns the printable representation of the origin kind.
func (o OriginKind) String() string {
	return string(o)
}

// Validate ensures that the origin is correct.
func (o OriginKind) Validate() error {
	if !knownOriginKinds[o] {
		return errors.NewNotValid(nil, "unknown origin")
	}
	return nil
}
