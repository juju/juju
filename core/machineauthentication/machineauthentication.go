// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineauthentication

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// ErrAuthenticationWorkerDying is used to indicate to callers that the
	// authentication worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrAuthenticationWorkerDying = errors.ConstError("authentication worker is dying")
)
