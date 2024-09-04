package errors

import "github.com/juju/errors"

const (
	// UnitNotFound describes an error that occurs when
	// the unit being operated on does not exist.
	UnitNotFound = errors.ConstError("unit not found")
)
