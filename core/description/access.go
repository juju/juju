// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

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

	// SuperUserAccess allows user unrestricted permissions in the subject.
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

// EqualOrGreaterAccessThan returns true if the provided access is equal or
// less than the current.
func (a Access) EqualOrGreaterAccessThan(access Access) bool {
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
	case AdminAccess:
		return access == ReadAccess ||
			access == WriteAccess
	// Controller permissions
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

// LessAccessThan returns true if the provided access is greater than
// the current.
func (a Access) LessAccessThan(access Access) bool {
	switch a {
	case UndefinedAccess:
		return access == ReadAccess ||
			access == WriteAccess ||
			access == AdminAccess ||
			access == LoginAccess ||
			access == AddModelAccess ||
			access == SuperuserAccess
	case ReadAccess:
		return access == WriteAccess ||
			access == AdminAccess
	case WriteAccess:
		return access == AdminAccess
	// Controller permissions
	case LoginAccess:
		return access == AddModelAccess ||
			access == SuperuserAccess
	case AddModelAccess:
		return access == SuperuserAccess
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
