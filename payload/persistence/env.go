// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"

	"github.com/juju/juju/payload"
)

// EnvPersistenceEntities provides all the information needed to produce
// a new EnvPersistence value.
type EnvPersistenceEntities interface {
	// AssignedMachineID the machine to which the identfies unit is assigned.
	AssignedMachineID(unitName string) (string, error)
}

// EnvPersistence provides the persistence functionality for the
// Juju environment as a whole.
type EnvPersistence struct {
	db *Persistence
	st EnvPersistenceEntities
}

// NewEnvPersistence wraps the "db" in a new EnvPersistence.
func NewEnvPersistence(db PersistenceBase, st EnvPersistenceEntities) *EnvPersistence {
	return &EnvPersistence{
		db: NewPersistence(db, ""),
		st: st,
	}
}

// ListAll returns the list of all payloads in the environment.
func (ep *EnvPersistence) ListAll() ([]payload.FullPayloadInfo, error) {
	logger.Tracef("listing all payloads")

	docs, err := ep.db.allModelPayloads()
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitMachines := make(map[string]string)
	var fullPayloads []payload.FullPayloadInfo
	for _, doc := range docs {
		machineID, ok := unitMachines[doc.UnitID]
		if !ok {
			machineID, err = ep.st.AssignedMachineID(doc.UnitID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			unitMachines[doc.UnitID] = machineID
		}
		fullPayloads = append(fullPayloads, payload.FullPayloadInfo{
			Payload: doc.payload(doc.UnitID),
			Machine: machineID,
		})
	}
	return fullPayloads, nil
}
