// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	"github.com/juju/errors"
)

const (
	// ErrorHTTPClientPurposeInvalid is an error that describes a problem where
	// a [HTTPClientPurpose] is not understood.
	ErrorHTTPClientPurposeInvalid = errors.ConstError("http client purpose is invalid")

	// ErrorHTTPClientForPurposeNotFound is an error that describes a problem
	// where a http client cannot be found for a [HTTPClientPurpose].
	ErrorHTTPClientForPurposeNotFound = errors.ConstError("http client for purpose not found")
)
