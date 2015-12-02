// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the params import if possible.

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
	internal "github.com/juju/juju/payload/api/private"
)

// UnitPayloads exposes the State functionality for a unit's payloads.
type UnitPayloads interface {
	// Track tracks a payload for the unit and info.
	Track(info payload.Payload) error
	// List returns information on the payload with the id on the unit.
	List(ids ...string) ([]payload.Result, error)
	// Settatus sets the status for the payload with the given id on the unit.
	SetStatus(id, status string) error
	// LookUp returns the payload ID for the given name/rawID pair.
	LookUp(name, rawID string) (string, error)
	// Untrack removes the information for the payload with the given id.
	Untrack(id string) error
}

// UnitFacade serves payload-specific API methods.
type UnitFacade struct {
	// State exposes the payload aspect of Juju's state.
	State UnitPayloads
}

// NewUnitFacade builds a new facade for the given State.
func NewUnitFacade(st UnitPayloads) *UnitFacade {
	return &UnitFacade{State: st}
}

// Track stores a payload to be tracked in state.
func (uf UnitFacade) Track(args internal.TrackArgs) (internal.PayloadResults, error) {
	logger.Debugf("tracking %d payloads from API", len(args.Payloads))

	var r internal.PayloadResults
	for _, apiPayload := range args.Payloads {
		pl, err := api.API2Payload(apiPayload)
		if err != nil {
			return r, errors.Trace(err)
		}
		logger.Debugf("tracking payload from API: %#v", pl)

		id, err := uf.track(pl.Payload)
		res := internal.NewPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

func (uf UnitFacade) track(pl payload.Payload) (string, error) {
	if err := uf.State.Track(pl); err != nil {
		return "", errors.Trace(err)
	}
	id, err := uf.State.LookUp(pl.Name, pl.ID)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

// List builds the list of payload being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// payloads for the unit are returned.
func (uf UnitFacade) List(args params.Entities) (internal.PayloadResults, error) {
	if len(args.Entities) == 0 {
		return uf.listAll()
	}

	var ids []string
	for _, entity := range args.Entities {
		id, err := internal.API2ID(entity.Tag)
		if err != nil {
			return internal.PayloadResults{}, errors.Trace(err)
		}
		ids = append(ids, id)
	}

	results, err := uf.State.List(ids...)
	if err != nil {
		return internal.PayloadResults{}, errors.Trace(err)
	}

	var r internal.PayloadResults
	for _, result := range results {
		res := internal.Result2api(result)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

func (uf UnitFacade) listAll() (internal.PayloadResults, error) {
	var r internal.PayloadResults

	results, err := uf.State.List()
	if err != nil {
		return r, errors.Trace(err)
	}

	for _, result := range results {
		pl := result.Payload
		id, err := uf.State.LookUp(pl.Name, pl.ID)
		if err != nil {
			logger.Errorf("failed to look up ID for %q: %v", pl.FullID(), err)
			id = ""
		}
		apipl := api.Payload2api(*pl)

		res := internal.NewPayloadResult(id, nil)
		res.Payload = &apipl
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// LookUp identifies the payload with the provided name and raw ID.
func (uf UnitFacade) LookUp(args internal.LookUpArgs) (internal.PayloadResults, error) {
	var r internal.PayloadResults
	for _, arg := range args.Args {
		id, err := uf.State.LookUp(arg.Name, arg.ID)
		res := internal.NewPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetStatus sets the raw status of a payload.
func (uf UnitFacade) SetStatus(args internal.SetStatusArgs) (internal.PayloadResults, error) {
	var r internal.PayloadResults
	for _, arg := range args.Args {
		id, err := internal.API2ID(arg.Tag)
		if err != nil {
			return r, errors.Trace(err)
		}

		err = uf.State.SetStatus(id, arg.Status)
		res := internal.NewPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// Untrack marks the identified payload as no longer being tracked.
func (uf UnitFacade) Untrack(args params.Entities) (internal.PayloadResults, error) {
	var r internal.PayloadResults
	for _, entity := range args.Entities {
		id, err := internal.API2ID(entity.Tag)
		if err != nil {
			return r, errors.Trace(err)
		}

		err = uf.State.Untrack(id)
		res := internal.NewPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}
