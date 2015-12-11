// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
)

// TODO(ericsnow) Would origins make sense in the charm metadata?

// These are the valid kinds of resource origin.
const (
	OriginKindUnknown OriginKind = ""
	OriginKindUpload  OriginKind = "upload"
)

var knownOriginKinds = map[OriginKind]bool{
	OriginKindUpload: true,
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

// Origin identifies where a resource came from.
type Origin struct {
	// Kind is the origin's kind.
	Kind OriginKind

	// Value is the specific origin.
	Value string
}

// String returns the printable representation of the origin.
func (o Origin) String() string {
	return o.Value
}

// Validate ensures that the origin is correct.
func (o Origin) Validate() error {
	if err := o.Kind.Validate(); err != nil {
		return errors.Annotate(err, "bad origin kind")
	}

	switch o.Kind {
	case OriginKindUpload:
		// Ensure it's a valid username.
		if o.Value == "" {
			return errors.NewNotValid(nil, "missing upload username")
		}
	}

	return nil
}
