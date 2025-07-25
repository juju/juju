// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names/v6"
)

// Controller encapsulates state for the Juju controller as a whole,
// as opposed to model specific functionality.
//
// This type is primarily used in the state.Initialize function, and
// in the yet to be hooked up controller worker.
type Controller struct {
	pool     *StatePool
	ownsPool bool
}

// NewController returns a controller object that doesn't own
// the state pool it has been given. This is for convenience
// at this time to get access to controller methods.
func NewController(pool *StatePool) *Controller {
	return &Controller{pool: pool}
}

// StatePool provides access to the state pool of the controller.
func (ctlr *Controller) StatePool() *StatePool {
	return ctlr.pool
}

// SystemState returns the State object for the controller model.
func (ctlr *Controller) SystemState() (*State, error) {
	return ctlr.pool.SystemState()
}

// Close the connection to the database.
func (ctlr *Controller) Close() error {
	if ctlr.ownsPool {
		ctlr.pool.Close()
	}
	return nil
}

// GetState returns a new State instance for the specified model. The
// connection uses the same credentials and policy as the Controller.
func (ctlr *Controller) GetState(modelTag names.ModelTag) (*PooledState, error) {
	return ctlr.pool.Get(modelTag.Id())
}

// ControllerInfo holds information about currently
// configured controller machines.
type ControllerInfo struct {
	// CloudName is the name of the cloud to which this controller is deployed.
	CloudName string
}

// ControllerInfo returns information about
// the currently configured controller machines.
func (st *State) ControllerInfo() (*ControllerInfo, error) {
	return &ControllerInfo{}, nil
}
