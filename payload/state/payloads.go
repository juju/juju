// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/payload"
)

// EnvPersistence provides the persistence functionality for the
// Juju environment as a whole.
type EnvPersistence interface {
	// ListAll returns the list of all payloads in the environment.
	ListAll() ([]payload.FullPayloadInfo, error)
}

// EnvPayloads provides the functionality related to an env's
// payloads, as needed by state.
type EnvPayloads struct {
	Persist EnvPersistence
}

// ListAll builds the list of payload information that is registered in state.
func (ps EnvPayloads) ListAll() ([]payload.FullPayloadInfo, error) {
	logger.Tracef("listing all payloads")

	payloads, err := ps.Persist.ListAll()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return payloads, nil
}
