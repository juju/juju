// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2/txn"

	stateerrors "github.com/juju/juju/state/errors"
)

var (
	newProviderIDNotUniqueError     = stateerrors.NewProviderIDNotUniqueError
	newParentDeviceHasChildrenError = stateerrors.NewParentDeviceHasChildrenError
	newErrCharmAlreadyUploaded      = stateerrors.NewErrCharmAlreadyUploaded
	newHasAssignedUnitsError        = stateerrors.NewHasAssignedUnitsError
	newHasContainersError           = stateerrors.NewHasContainersError
	newHasAttachmentsError          = stateerrors.NewHasAttachmentsError
	newHasHostedModelsError         = stateerrors.NewHasHostedModelsError
	newHasPersistentStorageError    = stateerrors.NewHasPersistentStorageError
	newModelNotEmptyError           = stateerrors.NewModelNotEmptyError
	newStorageAttachedError         = stateerrors.NewStorageAttachedError
	newDeletedUserError             = stateerrors.NewDeletedUserError
	newNeverLoggedInError           = stateerrors.NewNeverLoggedInError
	newNeverConnectedError          = stateerrors.NewNeverConnectedError
	newVersionInconsistentError     = stateerrors.NewVersionInconsistentError

	IsCharmAlreadyUploadedError    = stateerrors.IsCharmAlreadyUploadedError
	IsProviderIDNotUniqueError     = stateerrors.IsProviderIDNotUniqueError
	IsParentDeviceHasChildrenError = stateerrors.IsParentDeviceHasChildrenError
	IsIncompatibleSeriesError      = stateerrors.IsIncompatibleSeriesError
	IsNotAlive                     = stateerrors.IsNotAlive
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
)

func onAbort(txnErr, err error) error {
	if txnErr == txn.ErrAborted ||
		errors.Cause(txnErr) == txn.ErrAborted {
		return errors.Trace(err)
	}
	return errors.Trace(txnErr)
}
