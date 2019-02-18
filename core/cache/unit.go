// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
)

// Unit represents an unit in a cached model.
type Unit struct {
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details    UnitChange
	configHash string
}

func newUnit(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Unit {
	u := &Unit{
		metrics: metrics,
		hub:     hub,
	}
	return u
}

func (u *Unit) setDetails(details UnitChange) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.details = details

	// TODO (manadart 2019-02-11): Maintain hash and publish changes.
}
