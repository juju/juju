// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import coremodel "github.com/juju/juju/core/model"

// UpdateCredentialModelResult holds details of a model
// which was affected by a credential update, and any
// errors encountered validating the credential.
type UpdateCredentialModelResult struct {
	// ModelUUID contains model's UUID.
	ModelUUID coremodel.UUID

	// ModelName contains model name.
	ModelName string

	// Errors contains the errors accumulated while trying to update a credential.
	Errors []error
}
