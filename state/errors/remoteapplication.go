// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// RemoteApplicationReplaceFailedError indicates that an in-place
	// replacement of an existing remote application (consuming a newer
	// version of an offer) failed while running the replacement's own
	// transactions. The operation that triggered the replacement did
	// not itself run, and may be retried once the underlying cause is
	// resolved; the cause is preserved in the error chain.
	// TODO: wire this up through the API layer.
	RemoteApplicationReplaceFailedError = errors.ConstError("remote application replace failed")
)
