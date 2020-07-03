// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"

	stateerrors "github.com/juju/juju/state/errors"
)

type (
	// TODO: remove once fixed all the other imports outside of state.
	ErrProviderIDNotUnique     = stateerrors.ErrProviderIDNotUnique
	ErrParentDeviceHasChildren = stateerrors.ErrParentDeviceHasChildren
	ErrIncompatibleSeries      = stateerrors.ErrIncompatibleSeries
)

var (
	// TODO: remove once fixed all the other imports outside of state.
	ErrCharmRevisionAlreadyModified = stateerrors.ErrCharmRevisionAlreadyModified
	ErrDead                         = stateerrors.ErrDead
	ErrCannotEnterScope             = stateerrors.ErrCannotEnterScope
	ErrCannotEnterScopeYet          = stateerrors.ErrCannotEnterScopeYet
	ErrUnitHasSubordinates          = stateerrors.ErrUnitHasSubordinates
	ErrUnitHasStorageAttachments    = stateerrors.ErrUnitHasStorageAttachments

	NewProviderIDNotUniqueError     = stateerrors.NewProviderIDNotUniqueError
	newParentDeviceHasChildrenError = stateerrors.NewParentDeviceHasChildrenError
	NewErrCharmAlreadyUploaded      = stateerrors.NewErrCharmAlreadyUploaded
	NewHasAssignedUnitsError        = stateerrors.NewHasAssignedUnitsError
	NewHasContainersError           = stateerrors.NewHasContainersError
	NewHasAttachmentsError          = stateerrors.NewHasAttachmentsError
	NewHasHostedModelsError         = stateerrors.NewHasHostedModelsError
	NewHasPersistentStorageError    = stateerrors.NewHasPersistentStorageError
	NewModelNotEmptyError           = stateerrors.NewModelNotEmptyError
	NewStorageAttachedError         = stateerrors.NewStorageAttachedError
	NewDeletedUserError             = stateerrors.NewDeletedUserError
	NewNeverLoggedInError           = stateerrors.NewNeverLoggedInError
	NewNeverConnectedError          = stateerrors.NewNeverConnectedError
	NewVersionInconsistentError     = stateerrors.NewVersionInconsistentError

	IsCharmAlreadyUploadedError    = stateerrors.IsCharmAlreadyUploadedError
	IsProviderIDNotUniqueError     = stateerrors.IsProviderIDNotUniqueError
	IsParentDeviceHasChildrenError = stateerrors.IsParentDeviceHasChildrenError
	IsIncompatibleSeriesError      = stateerrors.IsIncompatibleSeriesError
	IsNotAlive                     = stateerrors.IsNotAlive
	IsUpgradeInProgressError       = stateerrors.IsUpgradeInProgressError
	IsHasAssignedUnitsError        = stateerrors.IsHasAssignedUnitsError
	IsHasContainersError           = stateerrors.IsHasContainersError
	IsHasAttachmentsError          = stateerrors.IsHasAttachmentsError
	IsHasHostedModelsError         = stateerrors.IsHasHostedModelsError
	IsHasPersistentStorageError    = stateerrors.IsHasPersistentStorageError
	IsModelNotEmptyError           = stateerrors.IsModelNotEmptyError
	IsStorageAttachedError         = stateerrors.IsStorageAttachedError
	IsDeletedUserError             = stateerrors.IsDeletedUserError
	IsNeverLoggedInError           = stateerrors.IsNeverLoggedInError
	IsNeverConnectedError          = stateerrors.IsNeverConnectedError
	IsVersionInconsistentError     = stateerrors.IsVersionInconsistentError
)

var (
	// State package internal errors.
	machineNotAliveErr     = stateerrors.NewNotAliveError("machine")
	applicationNotAliveErr = stateerrors.NewNotAliveError("application")
	unitNotAliveErr        = stateerrors.NewNotAliveError("unit")
	spaceNotAliveErr       = stateerrors.NewNotAliveError("space")
	subnetNotAliveErr      = stateerrors.NewNotAliveError("subnet")
	notAliveErr            = stateerrors.NewNotAliveError("")
	errUpgradeInProgress   = stateerrors.ErrUpgradeInProgress
)

func onAbort(txnErr, err error) error {
	if txnErr == txn.ErrAborted ||
		errors.Cause(txnErr) == txn.ErrAborted {
		return errors.Trace(err)
	}
	return errors.Trace(txnErr)
}
