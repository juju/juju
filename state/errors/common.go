// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

var (

	// ErrCannotEnterScope indicates that a relation unit failed to enter its scope
	// due to either the unit or the relation not being Alive.
	ErrCannotEnterScope = stderrors.New("cannot enter scope: unit or relation is not alive")

	// ErrCannotEnterScopeYet indicates that a relation unit failed to enter its
	// scope due to a required and pre-existing subordinate unit that is not Alive.
	// Once that subordinate has been removed, a new one can be created.
	ErrCannotEnterScopeYet = stderrors.New("cannot enter scope yet: non-alive subordinate unit has not been removed")
)

// errCharmAlreadyUploaded is returned by UpdateUploadedCharm() when
// the given charm is already uploaded and marked as not pending in
// state.
type errCharmAlreadyUploaded struct {
	curl *charm.URL
}

func NewErrCharmAlreadyUploaded(curl *charm.URL) error {
	return &errCharmAlreadyUploaded{curl: curl}
}

func (e *errCharmAlreadyUploaded) Error() string {
	return fmt.Sprintf("charm %q already uploaded", e.curl)
}

// IsCharmAlreadyUploadedError returns if the given error is
// errCharmAlreadyUploaded.
func IsCharmAlreadyUploadedError(err interface{}) bool {
	if err == nil {
		return false
	}
	// In case of a wrapped error, check the cause first.
	value := err
	cause := errors.Cause(err.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*errCharmAlreadyUploaded)
	return ok
}

// ErrCharmRevisionAlreadyModified is returned when a pending or
// placeholder charm is no longer pending or a placeholder, signaling
// the charm is available in state with its full information.
var ErrCharmRevisionAlreadyModified = fmt.Errorf("charm revision already modified")

var ErrDead = fmt.Errorf("not found or dead")

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

var (
	machineNotAliveErr     = notAliveError{"machine"}
	applicationNotAliveErr = notAliveError{"application"}
	unitNotAliveErr        = notAliveError{"unit"}
	spaceNotAliveErr       = notAliveError{"space"}
	subnetNotAliveErr      = notAliveError{"subnet"}
	notAliveErr            = notAliveError{""}
)

func onAbort(txnErr, err error) error {
	if txnErr == txn.ErrAborted ||
		errors.Cause(txnErr) == txn.ErrAborted {
		return errors.Trace(err)
	}
	return errors.Trace(txnErr)
}

// ErrProviderIDNotUnique is a standard error to indicate the value specified
// for a ProviderID field is not unique within the current model.
type ErrProviderIDNotUnique struct {
	duplicateIDs []string
}

func (e *ErrProviderIDNotUnique) Error() string {
	idList := strings.Join(e.duplicateIDs, ", ")
	return fmt.Sprintf("provider IDs not unique: %s", idList)
}

// NewProviderIDNotUniqueError returns an instance of ErrProviderIDNotUnique
// initialized with the given duplicate provider IDs.
func NewProviderIDNotUniqueError(providerIDs ...network.Id) error {
	stringIDs := make([]string, len(providerIDs))
	for i, providerID := range providerIDs {
		stringIDs[i] = string(providerID)
	}
	return newProviderIDNotUniqueErrorFromStrings(stringIDs)
}

func newProviderIDNotUniqueErrorFromStrings(providerIDs []string) error {
	return &ErrProviderIDNotUnique{
		duplicateIDs: providerIDs,
	}
}

// IsProviderIDNotUniqueError returns if the given error or its cause is
// ErrProviderIDNotUnique.
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
	_, ok := value.(*ErrProviderIDNotUnique)
	return ok
}

// ErrParentDeviceHasChildren is a standard error to indicate a network
// link-layer device cannot be removed because other existing devices refer to
// it as their parent.
type ErrParentDeviceHasChildren struct {
	parentName  string
	numChildren int
}

func (e *ErrParentDeviceHasChildren) Error() string {
	return fmt.Sprintf("parent device %q has %d children", e.parentName, e.numChildren)
}

func NewParentDeviceHasChildrenError(parentName string, numChildren int) error {
	return &ErrParentDeviceHasChildren{
		parentName:  parentName,
		numChildren: numChildren,
	}
}

// IsParentDeviceHasChildrenError returns if the given error or its cause is
// ErrParentDeviceHasChildren.
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
	_, ok := value.(*ErrParentDeviceHasChildren)
	return ok
}

// ErrIncompatibleSeries is a standard error to indicate that the series
// requested is not compatible with the charm of the application.
type ErrIncompatibleSeries struct {
	SeriesList []string
	Series     string
	CharmName  string
}

func (e *ErrIncompatibleSeries) Error() string {
	return fmt.Sprintf("series %q not supported by charm %q, supported series are: %s",
		e.Series, e.CharmName, strings.Join(e.SeriesList, ", "))
}

// IsIncompatibleSeriesError returns if the given error or its cause is
// ErrIncompatibleSeries.
func IsIncompatibleSeriesError(err interface{}) bool {
	if err == nil {
		return false
	}
	// In case of a wrapped error, check the cause first.
	value := err
	cause := errors.Cause(err.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*ErrIncompatibleSeries)
	return ok
}

var ErrUpgradeInProgress = errors.New("upgrade in progress")

// IsUpgradeInProgressError returns true if the error is caused by an
// in-progress upgrade.
func IsUpgradeInProgressError(err error) bool {
	return errors.Cause(err) == ErrUpgradeInProgress
}

// versionInconsistentError indicates one or more agents have a
// different version from the current one (even empty, when not yet
// set).
type versionInconsistentError struct {
	currentVersion version.Number
	agents         []string
}

// NewVersionInconsistentError returns a new instance of
// versionInconsistentError.
func NewVersionInconsistentError(currentVersion version.Number, agents []string) *versionInconsistentError {
	return &versionInconsistentError{currentVersion: currentVersion, agents: agents}
}

func (e *versionInconsistentError) Error() string {
	sort.Strings(e.agents)
	return fmt.Sprintf("some agents have not upgraded to the current model version %s: %s", e.currentVersion, strings.Join(e.agents, ", "))
}

// IsVersionInconsistentError returns if the given error is
// versionInconsistentError.
func IsVersionInconsistentError(e interface{}) bool {
	value := e
	// In case of a wrapped error, check the cause first.
	cause := errors.Cause(e.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*versionInconsistentError)
	return ok
}
