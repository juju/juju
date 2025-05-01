// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
)

const (
	// ErrCharmRevisionAlreadyModified is returned when a pending or
	// placeholder charm is no longer pending or a placeholder, signaling
	// the charm is available in state with its full information.
	ErrCharmRevisionAlreadyModified = errors.ConstError("charm revision already modified")

	ErrDead = errors.ConstError("not found or dead")

	// IncompatibleBaseError indicates the base selected is not supported by
	// the charm.
	IncompatibleBaseError = errors.ConstError("incompatible base for charm")
)

type notAliveError struct {
	entity string
}

func NewNotAliveError(entity string) error {
	return &notAliveError{entity: entity}
}

func (e notAliveError) Error() string {
	if e.entity == "" {
		return "not found or not alive"
	}
	return fmt.Sprintf("%v is not found or not alive", e.entity)
}

// IsNotAlive returns true if err is cause by a not alive error.
func IsNotAlive(err error) bool {
	_, ok := errors.Cause(err).(*notAliveError)
	return ok
}

// errProviderIDNotUnique is a standard error to indicate the value specified
// for a ProviderID field is not unique within the current model.
type errProviderIDNotUnique struct {
	duplicateIDs []string
}

func (e *errProviderIDNotUnique) Error() string {
	idList := strings.Join(e.duplicateIDs, ", ")
	return fmt.Sprintf("provider IDs not unique: %s", idList)
}

// NewProviderIDNotUniqueError returns an instance of errProviderIDNotUnique
// initialized with the given duplicate provider IDs.
func NewProviderIDNotUniqueError(providerIDs ...network.Id) error {
	stringIDs := make([]string, len(providerIDs))
	for i, providerID := range providerIDs {
		stringIDs[i] = string(providerID)
	}
	return newProviderIDNotUniqueErrorFromStrings(stringIDs)
}

func newProviderIDNotUniqueErrorFromStrings(providerIDs []string) error {
	return &errProviderIDNotUnique{
		duplicateIDs: providerIDs,
	}
}

// IsProviderIDNotUniqueError returns if the given error or its cause is
// errProviderIDNotUnique.
func IsProviderIDNotUniqueError(err interface{}) bool {
	if err == nil {
		return false
	}
	// In case of a wrapped error, check the cause first.
	value := err
	cause := errors.Cause(err.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*errProviderIDNotUnique)
	return ok
}

// errParentDeviceHasChildren is a standard error to indicate a network
// link-layer device cannot be removed because other existing devices refer to
// it as their parent.
type errParentDeviceHasChildren struct {
	parentName  string
	numChildren int
}

func (e *errParentDeviceHasChildren) Error() string {
	return fmt.Sprintf("parent device %q has %d children", e.parentName, e.numChildren)
}

func NewParentDeviceHasChildrenError(parentName string, numChildren int) error {
	return &errParentDeviceHasChildren{
		parentName:  parentName,
		numChildren: numChildren,
	}
}

// IsParentDeviceHasChildrenError returns if the given error or its cause is
// errParentDeviceHasChildren.
func IsParentDeviceHasChildrenError(err interface{}) bool {
	if err == nil {
		return false
	}
	// In case of a wrapped error, check the cause first.
	value := err
	cause := errors.Cause(err.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*errParentDeviceHasChildren)
	return ok
}
