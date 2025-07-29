// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// Controller encapsulates state for the Juju controller as a whole,
// as opposed to model specific functionality.
//
// This type is primarily used in the state.Initialize function, and
// in the yet to be hooked up controller worker.
type Controller struct {
}

// NewController returns a controller object that doesn't own
// the state pool it has been given. This is for convenience
// at this time to get access to controller methods.
func NewController() *Controller {
	return &Controller{}
}
