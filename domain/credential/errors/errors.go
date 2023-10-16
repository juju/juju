// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// CredentialValidation describes an error that occurs when a credential
	// cannot be validated for one or more models.
	CredentialValidation = errors.ConstError("credential is not valid for one or more models")
)
