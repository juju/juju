// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"

	"github.com/juju/juju/payload"
)

// EnvPersistence provides the persistence functionality for the
// Juju environment as a whole.
type EnvPersistence struct {
	db *Persistence
}

// NewEnvPersistence wraps the "db" in a new EnvPersistence.
func NewEnvPersistence(db PersistenceBase) *EnvPersistence {
	return &EnvPersistence{
		db: NewPersistence(db, ""),
	}
}

// ListAll returns the list of all payloads in the environment.
func (ep *EnvPersistence) ListAll() ([]payload.FullPayloadInfo, error) {
	logger.Tracef("listing all payloads")

	docs, err := ep.db.allModelPayloads()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fullPayloads []payload.FullPayloadInfo
	for _, doc := range docs {
		p := doc.payload()
		fullPayloads = append(fullPayloads, p)
	}
	return fullPayloads, nil
}
