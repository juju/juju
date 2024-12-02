// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// CredentialModelValidation describes an error that occurs when a credential
	// cannot be validated for one or more models.
	CredentialModelValidation = errors.ConstError("credential is not valid for one or more models")

	// NotFound describes an error that occurs when a cloud credential cannot be
	// found.
	NotFound = errors.ConstError("credential not found")

	// UnknownCloud describes an error that occurs when a credential for cloud
	// not known to the controller is updated.
	UnknownCloud = errors.ConstError("unknown cloud")

	// UserNotFound describes an error that occurs when a user is not found.
	UserNotFound = errors.ConstError("user not found")

	// CredentialNotFound describes an error that occurs when a credential is not found.
	CredentialNotFound = errors.ConstError("credential not found")
)
