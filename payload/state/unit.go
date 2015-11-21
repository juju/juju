// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/persistence"
)

var logger = loggo.GetLogger("juju.payload.state")

// TODO(ericsnow) We need a worker to clean up dying payloads.

// The persistence methods needed for payloads in state.
type payloadsPersistence interface {
	Track(id string, info payload.Payload) (bool, error)
	// SetStatus updates the status for a payload.
	SetStatus(id, status string) (bool, error)
	List(ids ...string) ([]payload.Payload, []string, error)
	ListAll() ([]payload.Payload, error)
	LookUp(name, rawID string) (string, error)
	Untrack(id string) (bool, error)
}

// UnitPayloads provides the functionality related to a unit's
// payloads, as needed by state.
type UnitPayloads struct {
	// Persist is the persistence layer that will be used.
	Persist payloadsPersistence

	// Unit identifies the unit associated with the payloads. This
	// is the "unit ID" of the targeted unit.
	Unit string

	// Machine identifies the unit's machine. This is the "machine ID"
	// of the machine on which the unit is running.
	Machine string

	newID func() (string, error)
}

// NewUnitPayloads builds a UnitPayloads for a unit.
func NewUnitPayloads(st persistence.PersistenceBase, unit, machine string) *UnitPayloads {
	persist := persistence.NewPersistence(st, unit)
	return &UnitPayloads{
		Persist: persist,
		Unit:    unit,
		Machine: machine,
		newID:   newID,
	}
}

func newID() (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "could not create new payload ID")
	}
	return uuid.String(), nil
}

// TODO(ericsnow) Return the new ID from Track()?

// Track inserts the provided payload info in state. The new Juju ID
// for the payload is returned.
func (uw UnitPayloads) Track(pl payload.Payload) error {
	logger.Tracef("tracking %#v", pl)

	if err := pl.Validate(); err != nil {
		return errors.NewNotValid(err, "bad payload")
	}

	id, err := uw.newID()
	if err != nil {
		return errors.Trace(err)
	}

	ok, err := uw.Persist.Track(id, pl)
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		return errors.NotValidf("payload %s (already in state)", id)
	}

	return nil
}

// SetStatus updates the raw status for the identified payload to the
// provided value.
func (uw UnitPayloads) SetStatus(id, status string) error {
	logger.Tracef("setting payload status for %q to %q", id, status)

	if err := payload.ValidateState(status); err != nil {
		return errors.Trace(err)
	}

	found, err := uw.Persist.SetStatus(id, status)
	if err != nil {
		return errors.Trace(err)
	}
	if !found {
		return errors.NotFoundf(id)
	}
	return nil
}

// List builds the list of payload information for the provided payload
// IDs. If none are provided then the list contains the info for all
// payloads associated with the unit. Missing payloads
// are ignored.
func (uw UnitPayloads) List(ids ...string) ([]payload.Result, error) {
	logger.Tracef("listing %v", ids)
	var err error
	var payloads []payload.Payload
	missingIDs := make(map[string]bool)
	if len(ids) == 0 {
		payloads, err = uw.Persist.ListAll()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _ = range payloads {
			ids = append(ids, "")
		}
	} else {
		var missing []string
		payloads, missing, err = uw.Persist.List(ids...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, id := range missing {
			missingIDs[id] = true
		}
	}

	var results []payload.Result
	i := 0
	for _, id := range ids {
		if missingIDs[id] {
			results = append(results, payload.Result{
				ID:       id,
				NotFound: true,
				Error:    errors.NotFoundf(id),
			})
			continue
		}
		pl := payloads[i]
		i += 1

		// TODO(ericsnow) Ensure that pl.Unit == uw.Unit?

		result := payload.Result{
			ID: id,
			Payload: &payload.FullPayloadInfo{
				Payload: pl,
				Machine: uw.Machine,
			},
		}
		if id == "" {
			// TODO(ericsnow) Do this more efficiently.
			id, err := uw.LookUp(pl.Name, pl.ID)
			if err != nil {
				id = ""
				result.Error = errors.Trace(err)
			}
			result.ID = id
		}
		results = append(results, result)
	}
	return results, nil
}

// LookUp returns the payload ID for the given name/rawID pair.
func (uw UnitPayloads) LookUp(name, rawID string) (string, error) {
	logger.Tracef("looking up payload id for %s/%s", name, rawID)

	id, err := uw.Persist.LookUp(name, rawID)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

// Untrack removes the identified payload from state. It does not
// trigger the actual destruction of the payload.
func (uw UnitPayloads) Untrack(id string) error {
	logger.Tracef("untracking %q", id)
	// If the record wasn't found then we're already done.
	_, err := uw.Persist.Untrack(id)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
