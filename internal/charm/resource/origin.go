// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
)

// These are the valid resource origins.
const (
	originUnknown Origin = iota
	OriginUpload
	OriginStore
)

var origins = map[Origin]string{
	OriginUpload: "upload",
	OriginStore:  "store",
}

// Origin identifies where a charm's resource comes from.
type Origin int

// ParseOrigin converts the provided string into an Origin.
// If it is not a known origin then an error is returned.
func ParseOrigin(value string) (Origin, error) {
	for o, str := range origins {
		if value == str {
			return o, nil
		}
	}
	return originUnknown, errors.Errorf("unknown origin %q", value)
}

// String returns the printable representation of the origin.
func (o Origin) String() string {
	return origins[o]
}

// Validate ensures that the origin is correct.
func (o Origin) Validate() error {
	// Ideally, only the (unavoidable) zero value would be invalid.
	// However, typedef'ing int means that the use of int literals
	// could result in invalid Type values other than the zero value.
	if _, ok := origins[o]; !ok {
		return errors.NewNotValid(nil, "unknown origin")
	}
	return nil
}
