// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
)

// canonicalPEN is the IANA-registered Private Enterprise Number
// assigned to Canonical. Among other things, this is used in RFC 5424
// structured data.
//
// See https://www.iana.org/assignments/enterprise-numbers/enterprise-numbers.
const canonicalPEN = 28978

// These are the recognized origin types.
const (
	OriginTypeUnknown OriginType = 0
	OriginTypeUser               = iota
	OriginTypeMachine
	OriginTypeUnit
)

var originTypes = map[OriginType]string{
	OriginTypeUnknown: "unknown",
	OriginTypeUser:    names.UserTagKind,
	OriginTypeMachine: names.MachineTagKind,
	OriginTypeUnit:    names.UnitTagKind,
}

// OriginType is the "enum" type for the different kinds of log record
// origin.
type OriginType int

// ParseOriginType converts a string to an OriginType or fails if
// not able. It round-trips with String().
func ParseOriginType(value string) (OriginType, error) {
	for ot, str := range originTypes {
		if value == str {
			return ot, nil
		}
	}
	const originTypeInvalid OriginType = -1
	return originTypeInvalid, errors.Errorf("unrecognized origin type %q", value)
}

// String returns a string representation of the origin type.
func (ot OriginType) String() string {
	return originTypes[ot]
}

// Validate ensures that the origin type is correct.
func (ot OriginType) Validate() error {
	// As noted above, typedef'ing int means that the use of int
	// literals or explicit type conversion could result in unsupported
	// "enum" values. Otherwise OriginType would not need this method.
	if _, ok := originTypes[ot]; !ok {
		return errors.NewNotValid(nil, "unsupported origin type")
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
			return errors.NewNotValid(nil, "bad user name")
		}
	case OriginTypeMachine:
		if !names.IsValidMachine(name) {
			return errors.NewNotValid(nil, "bad machine name")
		}
	case OriginTypeUnit:
		if !names.IsValidUnit(name) {
			return errors.NewNotValid(nil, "bad unit name")
		}
	}
	return nil
}

// Origin describes what created the record.
type Origin struct {
	// ControllerUUID is the ID of the Juju controller under which the
	// record originated.
	ControllerUUID string

	// ModelUUID is the ID of the Juju model under which the record
	// originated.
	ModelUUID string

	// Type identifies the kind of thing that generated the record.
	Type OriginType

	// Name identifies the thing that generated the record.
	Name string

	// JujuVersion is the version of the running Juju agent under which
	// the record originated.
	JujuVersion version.Number
}

// PrivateEnterpriseNumber returns the IANA-registered "SMI Network
// Management Private Enterprise Code" to use for the log record.
//
// See https://tools.ietf.org/html/rfc5424#section-7.2.2.
func (o Origin) PrivateEnterpriseNumber() int {
	return canonicalPEN
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
		return errors.NewNotValid(nil, fmt.Sprintf("ControllerUUID %q not a valid UUID", o.ControllerUUID))
	}

	if o.ModelUUID == "" {
		return errors.NewNotValid(nil, "empty ModelUUID")
	}
	if !names.IsValidModel(o.ModelUUID) {
		return errors.NewNotValid(nil, fmt.Sprintf("ModelUUID %q not a valid UUID", o.ModelUUID))
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
