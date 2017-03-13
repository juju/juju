// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// TrackPayloadArgs are the arguments for the
// PayloadsHookContext.Track endpoint.
type TrackPayloadArgs struct {
	// Payloads is the list of Payloads to track
	Payloads []Payload `json:"payloads"`
}

// LookUpPayloadArgs are the arguments for the LookUp endpoint.
type LookUpPayloadArgs struct {
	// Args is the list of arguments to pass to this function.
	Args []LookUpPayloadArg `json:"args"`
}

// LookUpPayloadArg contains all the information necessary to identify
// a payload.
type LookUpPayloadArg struct {
	// Name is the payload name.
	Name string `json:"name"`

	// ID uniquely identifies the payload for the given name.
	ID string `json:"id"`
}

// SetPayloadStatusArgs are the arguments for the
// PayloadsHookContext.SetStatus endpoint.
type SetPayloadStatusArgs struct {
	// Args is the list of arguments to pass to this function.
	Args []SetPayloadStatusArg `json:"args"`
}

// SetPayloadStatusArg are the arguments for a single call to the
// SetStatus endpoint.
type SetPayloadStatusArg struct {
	Entity

	// Status is the new status of the payload.
	Status string `json:"status"`
}

// PayloadResults is the result for a call that makes one or more
// requests about payloads.
type PayloadResults struct {
	Results []PayloadResult `json:"results"`
}

// PayloadResult contains the result for a single call.
type PayloadResult struct {
	Entity

	// Payload holds the details of the payload, if any.
	Payload *Payload `json:"payload"`

	// NotFound indicates that the payload was not found in state.
	NotFound bool `json:"not-found"`

	// Error is the error (if any) for the call referring to ID.
	Error *Error `json:"error,omitempty"`
}
