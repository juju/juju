// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"github.com/juju/errors"
)

// TODO(ericsnow) Would origins make sense in the charm metadata?

// These are the valid resource origins.
const (
	OriginUnknown Origin = ""
	OriginUpload  Origin = "upload"
)

var knownOrigins = map[Origin]bool{
	OriginUpload: true,
}

// Origin identifies the type of a resource origin.
type Origin string

// ParseOrigin converts the provided string into an Origin. If it is
// not a known origin then false is returned.
func ParseOrigin(value string) (Origin, bool) {
	o := Origin(value)
	return o, knownOrigins[o]
}

// String returns the printable representation of the origin.
func (o Origin) String() string {
	return string(o)
}

// Validate ensures that the origin is correct.
func (o Origin) Validate() error {
	if !knownOrigins[o] {
		return errors.NewNotValid(nil, "unknown origin")
	}
	return nil
}
