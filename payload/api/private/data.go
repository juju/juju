// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package private

import "github.com/juju/juju/apiserver/params"

// TrackArgs are the arguments for the Track endpoint.
type TrackArgs struct {
	// Payloads is the list of Payloads to track
	Payloads []params.Payload `json:"payloads"`
}

// List uses params.Entities.

// LookUpArgs are the arguments for the LookUp endpoint.
type LookUpArgs struct {
	// Args is the list of arguments to pass to this function.
	Args []LookUpArg `json:"args"`
}

// LookUpArg contains all the information necessary to identify a payload.
type LookUpArg struct {
	// Name is the payload name.
	Name string `json:"name"`
	// ID uniquely identifies the payload for the given name.
	ID string `json:"id"`
}

// SetStatusArgs are the arguments for the SetStatus endpoint.
type SetStatusArgs struct {
	// Args is the list of arguments to pass to this function.
	Args []SetStatusArg `json:"args"`
}

// SetStatusArg are the arguments for a single call to the
// SetStatus endpoint.
type SetStatusArg struct {
	params.Entity
	// Status is the new status of the payload.
	Status string `json:"status"`
}

// Untrack uses params.Entities.

// PayloadResults is the result for a call that makes one or more requests
// about payloads.
type PayloadResults struct {
	Results []PayloadResult `json:"results"`
}

// TODO(ericsnow) Eliminate the NotFound field?

// PayloadResult contains the result for a single call.
type PayloadResult struct {
	params.Entity
	// Payload holds the details of the payload, if any.
	Payload *params.Payload `json:"payload"`
	// NotFound indicates that the payload was not found in state.
	NotFound bool `json:"not-found"`
	// Error is the error (if any) for the call referring to ID.
	Error *params.Error `json:"error,omitempty"`
}
