// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/apiserver/common/errors"
)

var (
	// TODO(ycliuhw): remove this alias of error import once refactored all the existing import of `github.com/juju/juju/apiserver/common.Err*`
	NotSupportedError        = errors.NotSupportedError
	NoAddressSetError        = errors.NoAddressSetError
	UnknownModelError        = errors.UnknownModelError
	IsDischargeRequiredError = errors.IsDischargeRequiredError
	IsUpgradeInProgressError = errors.IsUpgradeInProgressError
	IsRedirectError          = errors.IsRedirectError

	ErrBadId              = errors.ErrBadId
	ErrBadCreds           = errors.ErrBadCreds
	ErrNoCreds            = errors.ErrNoCreds
	ErrLoginExpired       = errors.ErrLoginExpired
	ErrPerm               = errors.ErrPerm
	ErrNotLoggedIn        = errors.ErrNotLoggedIn
	ErrUnknownWatcher     = errors.ErrUnknownWatcher
	ErrStoppedWatcher     = errors.ErrStoppedWatcher
	ErrBadRequest         = errors.ErrBadRequest
	ErrTryAgain           = errors.ErrTryAgain
	ErrActionNotAvailable = errors.ErrActionNotAvailable

	OperationBlockedError = errors.OperationBlockedError
	ServerErrorAndStatus  = errors.ServerErrorAndStatus
	ServerError           = errors.ServerError
	DestroyErr            = errors.DestroyErr
	RestoreError          = errors.RestoreError
)

type (
	DischargeRequiredError = errors.DischargeRequiredError
	RedirectError          = errors.RedirectError
)
