// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

const (
	// ErrChangeStreamDying is used to indicate to *third parties* that the
	// change-stream worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrChangeStreamDying = errors.ConstError("change-stream worker is dying")

	// ErrEventMultiplexerDying is used to indicate to *third parties* that the
	// event multiplexer worker is dying, instead of catacomb.ErrDying, which
	// is unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrEventMultiplexerDying = errors.ConstError("event multiplexer worker is dying")
)

// ShortNamespace returns a short version of the namespace.
// If the namespace is the controller namespace, then it returns the
// controller namespace. Otherwise, it returns a short version of the model UUID.
func ShortNamespace(namespace string) string {
	if namespace == ControllerNS {
		return ControllerNS
	}
	return model.ShortModelUUID(model.UUID(namespace))
}
