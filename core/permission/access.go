// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
)

// Access represents a level of access.
type Access string

const (
	// NoAccess allows a user no permissions at all.
	NoAccess Access = ""

	// Model Permissions

	// ReadAccess allows a user to read information about a permission subject,
	// without being able to make any changes.
	ReadAccess Access = "read"

	// WriteAccess allows a user to make changes to a permission subject.
	WriteAccess Access = "write"

	// ConsumeAccess allows a user to consume a permission subject.
	ConsumeAccess Access = "consume"

	// AdminAccess allows a user full control over the subject.
	AdminAccess Access = "admin"

	// Controller permissions

	// LoginAccess allows a user to log-ing into the subject.
	LoginAccess Access = "login"

	// AddModelAccess allows user to add new models in subjects supporting it.
	AddModelAccess Access = "add-model"

	// SuperuserAccess allows user unrestricted permissions in the subject.
	SuperuserAccess Access = "superuser"
)

var AllAccessLevels = []Access{NoAccess, ReadAccess, WriteAccess, ConsumeAccess, AdminAccess, LoginAccess, AddModelAccess, SuperuserAccess}

// Validate returns error if the current is not a valid access level.
func (a Access) Validate() error {
	switch a {
	case NoAccess, AdminAccess, ReadAccess, WriteAccess,
		LoginAccess, AddModelAccess, SuperuserAccess, ConsumeAccess:
		return nil
	}
	return errors.NotValidf("access level %s", a)
}

// ObjectType is the type of the permission object/
type ObjectType int

// These values must match the values in the permission_object_type table.
const (
	Cloud ObjectType = iota
	Controller
	Model
	Offer
)

// Validate returns an error if the object type is not in the
// list of valid object types above.
func (o ObjectType) Validate() error {
	switch o {
	case Cloud, Controller, Model, Offer:
	default:
		return errors.NotValidf("object type")
	}
	return nil
}

// ID identifies the object of a permission, its key and type. Keys
// are names or uuid depending on the type.
type ID struct {
	ObjectType ObjectType
	Key        string
}

// Validate returns an error if the key is empty and/or the ObjectType
// is not in the list.
func (i ID) Validate() error {
	if i.Key == "" {
		return errors.NotValidf("empty key")
	}
	return i.ObjectType.Validate()
}

// ValidateAccess validates the access value is valid for this ID.
func (i ID) ValidateAccess(access Access) error {
	var err error
	switch i.ObjectType {
	case Cloud:
		err = ValidateCloudAccess(access)
	case Controller:
		err = ValidateControllerAccess(access)
	case Model:
		err = ValidateModelAccess(access)
	case Offer:
		err = ValidateOfferAccess(access)
	default:
		err = errors.NotValidf("access type %q", i.ObjectType)
	}
	return err
}

// ParseTagForID returns an ID of a permission object and must
// conform to the know object types.
func ParseTagForID(tag names.Tag) (ID, error) {
	if tag == nil {
		return ID{}, errors.BadRequestf("nil tag")
	}
	id := ID{Key: tag.Id()}
	switch tag.Kind() {
	case names.CloudTagKind:
		id.ObjectType = Cloud
	case names.ControllerTagKind:
		id.ObjectType = Controller
	case names.ModelTagKind:
		id.ObjectType = Model
	case names.ApplicationOfferTagKind:
		id.ObjectType = Offer
	default:
		return id, errors.NotValidf("target tag type %s", tag.Kind())
	}
	return id, nil
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

// ValidateOfferAccess returns error if the passed access is not a valid
// offer access level.
func ValidateOfferAccess(access Access) error {
	switch access {
	case ReadAccess, ConsumeAccess, AdminAccess:
		return nil
	}
	return errors.NotValidf("%q offer access", access)
}

// ValidateCloudAccess returns error if the passed access is not a valid
// cloud access level.
func ValidateCloudAccess(access Access) error {
	switch access {
	case AddModelAccess, AdminAccess:
		return nil
	}
	return errors.NotValidf("%q cloud access", access)
}

// ValidateControllerAccess returns error if the passed access is not a valid
// controller access level.
func ValidateControllerAccess(access Access) error {
	switch access {
	case LoginAccess, SuperuserAccess:
		return nil
	}
	return errors.NotValidf("%q controller access", access)
}

func (a Access) controllerValue() int {
	switch a {
	case NoAccess:
		return 0
	case LoginAccess:
		return 1
	case SuperuserAccess:
		return 2
	default:
		return -1
	}
}

func (a Access) cloudValue() int {
	switch a {
	case AddModelAccess:
		return 0
	case AdminAccess:
		return 1
	default:
		return -1
	}
}

func (a Access) modelValue() int {
	switch a {
	case NoAccess:
		return 0
	case ReadAccess:
		return 1
	case WriteAccess:
		return 2
	case AdminAccess:
		return 3
	default:
		return -1
	}
}

// EqualOrGreaterModelAccessThan returns true if the current access is equal
// or greater than the passed in access level.
func (a Access) EqualOrGreaterModelAccessThan(access Access) bool {
	v1, v2 := a.modelValue(), access.modelValue()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 >= v2
}

// GreaterModelAccessThan returns true if the current access is greater than
// the passed in access level.
func (a Access) GreaterModelAccessThan(access Access) bool {
	v1, v2 := a.modelValue(), access.modelValue()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 > v2
}

// EqualOrGreaterControllerAccessThan returns true if the current access is
// equal or greater than the passed in access level.
func (a Access) EqualOrGreaterControllerAccessThan(access Access) bool {
	v1, v2 := a.controllerValue(), access.controllerValue()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 >= v2
}

// GreaterControllerAccessThan returns true if the current access is
// greater than the passed in access level.
func (a Access) GreaterControllerAccessThan(access Access) bool {
	v1, v2 := a.controllerValue(), access.controllerValue()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 > v2
}

// EqualOrGreaterCloudAccessThan returns true if the current access is
// equal or greater than the passed in access level.
func (a Access) EqualOrGreaterCloudAccessThan(access Access) bool {
	v1, v2 := a.cloudValue(), access.cloudValue()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 >= v2
}

func (a Access) offerValue() int {
	switch a {
	case NoAccess:
		return 0
	case ReadAccess:
		return 1
	case ConsumeAccess:
		return 2
	case AdminAccess:
		return 3
	default:
		return -1
	}
}

// EqualOrGreaterOfferAccessThan returns true if the current access is
// equal or greater than the passed in access level.
func (a Access) EqualOrGreaterOfferAccessThan(access Access) bool {
	v1, v2 := a.offerValue(), access.offerValue()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 >= v2
}

// GreaterOfferAccessThan returns true if the current access is
// greater than the passed in access level.
func (a Access) GreaterOfferAccessThan(access Access) bool {
	v1, v2 := a.offerValue(), access.offerValue()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 > v2
}
