// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// Access represents a level of access.
type Access string

const (
	// UndefinedAccess is not a valid access type. It is the value
	// used when access is not defined at all.
	UndefinedAccess Access = ""

	// Model Permissions

	// ReadAccess allows a user to read information about a permission subject,
	// without being able to make any changes.
	ReadAccess Access = "read"

	// WriteAccess allows a user to make changes to a permission subject.
	WriteAccess Access = "write"

	// AdminAccess allows a user full control over the subject.
	AdminAccess Access = "admin"

	// Controller permissions

	// LoginAccess allows a user to log-ing into the subject.
	LoginAccess Access = "login"

	// AddModelAccess allows user to add new models in subjects supporting it.
	AddModelAccess Access = "addmodel"

	// SuperuserAccess allows user unrestricted permissions in the subject.
	SuperuserAccess Access = "superuser"
)

// Validate returns error if the current is not a valid access level.
func (a Access) Validate() error {
	switch a {
	case UndefinedAccess, AdminAccess, ReadAccess, WriteAccess,
		LoginAccess, AddModelAccess, SuperuserAccess:
		return nil
	}
	return errors.NotValidf("access level %s", a)
}

// ValidateModelAccess returns error if the passed access is not a valid
// model access level.
func ValidateModelAccess(access Access) error {
	switch access {
	case ReadAccess, WriteAccess, AdminAccess:
		return nil
	}
	return errors.NotValidf("%q model access", access)
}

//ValidateControllerAccess returns error if the passed access is not a valid
// controller access level.
func ValidateControllerAccess(access Access) error {
	switch access {
	case LoginAccess, AddModelAccess, SuperuserAccess:
		return nil
	}
	return errors.NotValidf("%q controller access", access)
}

// EqualOrGreaterModelAccessThan returns true if the provided access is equal or
// less than the current.
func (a Access) EqualOrGreaterModelAccessThan(access Access) bool {
	if a == access {
		return true
	}
	switch a {
	case UndefinedAccess:
		return false
	case ReadAccess:
		return access == UndefinedAccess
	case WriteAccess:
		return access == ReadAccess ||
			access == UndefinedAccess
	case AdminAccess, SuperuserAccess:
		return access == ReadAccess ||
			access == WriteAccess
	}
	return false
}

// EqualOrGreaterControllerAccessThan returns true if the provided access is equal or
// less than the current.
func (a Access) EqualOrGreaterControllerAccessThan(access Access) bool {
	if a == access {
		return true
	}
	switch a {
	case UndefinedAccess:
		return false
	case LoginAccess:
		return access == UndefinedAccess
	case AddModelAccess:
		return access == UndefinedAccess ||
			access == LoginAccess
	case SuperuserAccess:
		return access == UndefinedAccess ||
			access == LoginAccess ||
			access == AddModelAccess
	}
	return false
}

// accessField returns a Checker that accepts a string value only
// and returns a valid Access or an error.
func accessField() schema.Checker {
	return accessC{}
}

type accessC struct{}

func (c accessC) Coerce(v interface{}, path []string) (interface{}, error) {
	s := schema.String()
	in, err := s.Coerce(v, path)
	if err != nil {
		return nil, err
	}
	access := Access(in.(string))
	if err := access.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return access, nil
}
