// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/version"
)

// See https://www.iana.org/assignments/enterprise-numbers/enterprise-numbers.
const canonicalIANAid = "28978"

// These are the recognized origin types.
const (
	originTypeInvalid OriginType = -1
	OriginTypeUnknown OriginType = 0
	OriginTypeUser               = iota
)

var originTypes = map[OriginType]string{
	OriginTypeUnknown: "unknown",
	OriginTypeUser:    names.UserTagKind,
}

// OriginType is the "enum" type for the different kinds of log record origin.
type OriginType int

// ParseOriginType converts a string to an OriginType or fails if
// not able. It round-trips with String().
func ParseOriginType(value string) (OriginType, error) {
	for ot, str := range originTypes {
		if value == str {
			return ot, nil
		}
	}
	return originTypeInvalid, errors.Errorf("unrecognized origin type %q", value)
}

// String returns a string representation of the origin type.
func (ot OriginType) String() string {
	return originTypes[ot]
}

// Validate ensures that the origin type is correct.
func (ot OriginType) Validate() error {
	// Ideally, only the (unavoidable) zero value would be invalid.
	// However, typedef'ing int means that the use of int literals
	// could result in invalid values other than the zero value.
	if _, ok := originTypes[ot]; !ok {
		return errors.NewNotValid(nil, "unknown origin type")
	}
	return nil
}

// ValidateName ensures that the given origin name is valid within the
// context of the origin type.
func (ot OriginType) ValidateName(name string) error {
	switch ot {
	case OriginTypeUnknown:
		if name != "" {
			return errors.NewNotValid(nil, "origin name must not be set if type is unknown")
		}
	case OriginTypeUser:
		if !names.IsValidUser(name) {
			return errors.NewNotValid(nil, "bad user")
		}
	}
	return nil
}

// Validate ensures that the origin is correct.
type Origin struct {
	ControllerUUID string
	ModelUUID      string
	Type           OriginType
	Name           string
	JujuVersion    version.Number
}

// EnterpriseID returns the IANA-registered "SMI Network Management
// Private Enterprise Code" to use for the log record.
// (see https://tools.ietf.org/html/rfc5424#section-7.2.2)
func (o Origin) EnterpriseID() string {
	return canonicalIANAid
}

// SofwareName identifies the software that generated the log message.
// It is unique within the context of the enterprise ID.
func (o Origin) SoftwareName() string {
	return "jujud"
}

// Validate ensures that the origin is correct.
func (o Origin) Validate() error {
	if o.ControllerUUID == "" {
		return errors.NewNotValid(nil, "empty ControllerUUID")
	}
	if !names.IsValidModel(o.ControllerUUID) {
		err := errors.NewNotValid(nil, "must be UUID")
		return errors.Annotatef(err, "invalid ControllerUUID %q", o.ControllerUUID)
	}

	if o.ModelUUID == "" {
		return errors.NewNotValid(nil, "empty ModelUUID")
	}
	if !names.IsValidModel(o.ModelUUID) {
		err := errors.NewNotValid(nil, "must be UUID")
		return errors.Annotatef(err, "invalid ModelUUID %q", o.ModelUUID)
	}

	if err := o.Type.Validate(); err != nil {
		return errors.Annotate(err, "invalid Type")
	}

	if o.Name == "" && o.Type != OriginTypeUnknown {
		return errors.NewNotValid(nil, "empty Name")
	}
	if err := o.Type.ValidateName(o.Name); err != nil {
		return errors.Annotatef(err, "invalid Name %q", o.Name)
	}

	if o.JujuVersion == version.Zero {
		return errors.NewNotValid(nil, "empty JujuVersion")
	}

	return nil
}
