// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
)

// Branch represents an active branch in a cached model.
type Branch struct {
	// Resident identifies the branch as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details    BranchChange
	configHash string
}

func newBranch(metrics *ControllerGauges, hub *pubsub.SimpleHub, res *Resident) *Branch {
	return &Branch{
		Resident: res,
		metrics:  metrics,
		hub:      hub,
	}
}

func (b *Branch) setDetails(details BranchChange) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If this is the first receipt of details, set the removal message.
	if b.removalMessage == nil {
		b.removalMessage = RemoveBranch{
			ModelUUID: details.ModelUUID,
			Name:      details.Name,
		}
	}

	b.setStale(false)

	// TODO (manadart 2019-05-29): Publish changes for config deltas and tracking units.

	b.details = details
}
