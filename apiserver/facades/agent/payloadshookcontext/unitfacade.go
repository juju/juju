// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloadshookcontext

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.payloadshookcontext")

// NewHookContextFacade returns a new payloads hook context facade for
// the State and Unit given. It is used for facade registration.
func NewHookContextFacade(st *state.State, unit *state.Unit) (interface{}, error) {
	up, err := st.UnitPayloads(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUnitFacade(up), nil
}

// UnitPayloadBackend exposes the State functionality for a unit's payloads.
type UnitPayloadBackend interface {
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
	backend UnitPayloadBackend
}

// NewUnitFacade builds a new facade for the given backend.
func NewUnitFacade(backend UnitPayloadBackend) *UnitFacade {
	return &UnitFacade{backend: backend}
}

// Track stores a payload to be tracked in state.
func (uf UnitFacade) Track(args params.TrackPayloadArgs) (params.PayloadResults, error) {
	logger.Debugf("tracking %d payloads from API", len(args.Payloads))

	var r params.PayloadResults
	for _, apiPayload := range args.Payloads {
		pl, err := api.API2Payload(apiPayload)
		if err != nil {
			return r, errors.Trace(err)
		}
		logger.Debugf("tracking payload from API: %#v", pl)

		id, err := uf.track(pl.Payload)
		res := newPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

func (uf UnitFacade) track(pl payload.Payload) (string, error) {
	if err := uf.backend.Track(pl); err != nil {
		return "", errors.Trace(err)
	}
	id, err := uf.backend.LookUp(pl.Name, pl.ID)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

// List builds the list of payload being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// payloads for the unit are returned.
func (uf UnitFacade) List(args params.Entities) (params.PayloadResults, error) {
	if len(args.Entities) == 0 {
		return uf.listAll()
	}

	var ids []string
	for _, entity := range args.Entities {
		id, err := api.API2ID(entity.Tag)
		if err != nil {
			return params.PayloadResults{}, errors.Trace(err)
		}
		ids = append(ids, id)
	}

	results, err := uf.backend.List(ids...)
	if err != nil {
		return params.PayloadResults{}, errors.Trace(err)
	}

	var r params.PayloadResults
	for _, result := range results {
		res := Result2api(result)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

func (uf UnitFacade) listAll() (params.PayloadResults, error) {
	var r params.PayloadResults

	results, err := uf.backend.List()
	if err != nil {
		return r, errors.Trace(err)
	}

	for _, result := range results {
		pl := result.Payload
		id, err := uf.backend.LookUp(pl.Name, pl.ID)
		if err != nil {
			logger.Errorf("failed to look up ID for %q: %v", pl.FullID(), err)
			id = ""
		}
		apipl := api.Payload2api(*pl)

		res := newPayloadResult(id, nil)
		res.Payload = &apipl
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// LookUp identifies the payload with the provided name and raw ID.
func (uf UnitFacade) LookUp(args params.LookUpPayloadArgs) (params.PayloadResults, error) {
	var r params.PayloadResults
	for _, arg := range args.Args {
		id, err := uf.backend.LookUp(arg.Name, arg.ID)
		res := newPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetStatus sets the raw status of a payload.
func (uf UnitFacade) SetStatus(args params.SetPayloadStatusArgs) (params.PayloadResults, error) {
	var r params.PayloadResults
	for _, arg := range args.Args {
		id, err := api.API2ID(arg.Tag)
		if err != nil {
			return r, errors.Trace(err)
		}

		err = uf.backend.SetStatus(id, arg.Status)
		res := newPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// Untrack marks the identified payload as no longer being tracked.
func (uf UnitFacade) Untrack(args params.Entities) (params.PayloadResults, error) {
	var r params.PayloadResults
	for _, entity := range args.Entities {
		id, err := api.API2ID(entity.Tag)
		if err != nil {
			return r, errors.Trace(err)
		}

		err = uf.backend.Untrack(id)
		res := newPayloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// newPayloadResult builds a new PayloadResult from the provided tag
// and error. NotFound is also set based on the error.
func newPayloadResult(id string, err error) params.PayloadResult {
	result := payload.Result{
		ID:       id,
		Payload:  nil,
		NotFound: errors.IsNotFound(err),
		Error:    err,
	}
	return Result2api(result)
}

// Result2api converts the payload.Result into a PayloadResult.
func Result2api(result payload.Result) params.PayloadResult {
	res := params.PayloadResult{
		NotFound: result.NotFound,
	}

	if result.ID != "" {
		res.Tag = names.NewPayloadTag(result.ID).String()
	}

	if result.Payload != nil {
		pl := api.Payload2api(*result.Payload)
		res.Payload = &pl
	}

	if result.Error != nil {
		res.Error = common.ServerError(result.Error)
	}

	return res
}
