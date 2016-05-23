// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/payload"
)

var logger = loggo.GetLogger("juju.payload.state")

// TODO(ericsnow) We need a worker to clean up dying payloads.

// Persistence exposes methods needed for payloads in state.
type Persistence interface {
	Track(info payload.FullPayloadInfo) error
	// SetStatus updates the status for a payload.
	SetStatus(name, status string) error
	List(names ...string) ([]payload.FullPayloadInfo, []string, error)
	ListAll() ([]payload.FullPayloadInfo, error)
	Untrack(name string) error
}

// UnitPayloads provides the functionality related to a unit's
// payloads, as needed by state.
type UnitPayloads struct {
	// Persist is the persistence layer that will be used.
	Persist Persistence

	// Unit identifies the unit associated with the payloads. This
	// is the "unit ID" of the targeted unit.
	Unit string

	// Machine identifies the unit's machine. This is the "machine ID"
	// of the machine on which the unit is running.
	Machine string
}

// NewUnitPayloads builds a UnitPayloads for a unit.
func NewUnitPayloads(persist Persistence, unit, machine string) *UnitPayloads {
	return &UnitPayloads{
		Persist: persist,
		Unit:    unit,
		Machine: machine,
	}
}

// Track inserts the provided payload info in state. If the payload
// is already in the DB then it is replaced.
func (uw UnitPayloads) Track(pl payload.Payload) error {
	logger.Tracef("tracking %#v", pl)

	// TODO(ericsnow) The unit should probably not be a part
	// of payload.Payload...
	//pl.Unit = uw.Unit

	if err := pl.Validate(); err != nil {
		return errors.NewNotValid(err, "bad payload")
	}

	if existing, err := uw.lookUp(pl.Name); err != nil {
		if errors.Cause(err) != payload.ErrNotFound {
			return errors.Trace(err)
		}
		// Wasn't found, so we're okay.
	} else {
		logger.Infof("payload %q (raw: %q) already exists; replacing...", pl.Name, existing.ID)
	}

	full := payload.FullPayloadInfo{
		Payload: pl,
		Machine: uw.Machine,
	}
	if err := uw.Persist.Track(full); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// SetStatus updates the raw status for the identified payload to the
// provided value. If the payload is missing then payload.ErrNotFound
// is returned.
func (uw UnitPayloads) SetStatus(name, status string) error {
	logger.Tracef("setting payload status for %q to %q", name, status)

	if err := payload.ValidateState(status); err != nil {
		return errors.Trace(err)
	}

	if err := uw.Persist.SetStatus(name, status); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// List builds the list of payload information for the provided payload
// IDs. If none are provided then the list contains the info for all
// payloads associated with the unit. Missing payloads
// are ignored.
func (uw UnitPayloads) List(names ...string) ([]payload.Result, error) {
	logger.Tracef("listing %v", names)
	var err error
	var payloads []payload.FullPayloadInfo
	missingIDs := make(map[string]bool)
	if len(names) == 0 {
		payloads, err = uw.Persist.ListAll()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _ = range payloads {
			names = append(names, "")
		}
	} else {
		var missing []string
		payloads, missing, err = uw.Persist.List(names...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, id := range missing {
			missingIDs[id] = true
		}
	}

	var results []payload.Result
	i := 0
	for _, name := range names {
		if missingIDs[name] {
			results = append(results, payload.Result{
				ID:       name,
				NotFound: true,
				Error:    errors.NotFoundf(name),
			})
			continue
		}
		pl := payloads[i]
		i += 1

		// TODO(ericsnow) Ensure that pl.Name == name?
		// TODO(ericsnow) Ensure that pl.Unit == uw.Unit?

		result := payload.Result{
			ID:      pl.Name,
			Payload: &pl,
		}
		results = append(results, result)
	}
	return results, nil
}

// TODO(ericsnow) Drop LookUp in favor of calling List() directly.

// LookUp returns the payload ID for the given name/rawID pair.
func (uw UnitPayloads) LookUp(name, rawID string) (string, error) {
	logger.Tracef("looking up payload id for %s/%s", name, rawID)
	pl, err := uw.lookUp(name)
	if err != nil {
		return "", errors.Trace(err)
	}
	return pl.Name, nil
}

func (uw UnitPayloads) lookUp(name string) (payload.FullPayloadInfo, error) {
	var pl payload.FullPayloadInfo

	results, err := uw.List(name)
	if err != nil {
		return pl, errors.Trace(err)
	}
	if results[0].NotFound {
		return pl, errors.Annotate(payload.ErrNotFound, name)
	}
	if results[0].Error != nil {
		return pl, errors.Trace(results[0].Error)
	}
	pl = *results[0].Payload

	return pl, nil
}

// Untrack removes the identified payload from state. It does not
// trigger the actual destruction of the payload. If the payload is
// missing then this is a noop.
func (uw UnitPayloads) Untrack(name string) error {
	logger.Tracef("untracking %q", name)
	// If the record wasn't found then we're already done.
	err := uw.Persist.Untrack(name)
	if err != nil && errors.Cause(err) != payload.ErrNotFound {
		return errors.Trace(err)
	}
	return nil
}
