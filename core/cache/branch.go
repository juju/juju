// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/pubsub"

	"github.com/juju/juju/core/settings"
)

const branchChange = "branch-change"

// Branch represents an active branch in a cached model.
type Branch struct {
	// Resident identifies the branch as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	metrics *ControllerGauges
	hub     *pubsub.SimpleHub

	details BranchChange
}

func newBranch(metrics *ControllerGauges, hub *pubsub.SimpleHub, res *Resident) *Branch {
	return &Branch{
		Resident: res,
		metrics:  metrics,
		hub:      hub,
	}
}

// Note that these property accessors are not lock-protected.
// They are intended for calling from external packages that have retrieved a
// deep copy from the cache.

// Name returns the name of the branch.
// It is guaranteed to uniquely identify an active branch in the cache.
func (b *Branch) Name() string {
	return b.details.Name
}

// AssignedUnits returns a map of the names of units tracking this branch,
// keyed by application names with changes made under the branch.
func (b *Branch) AssignedUnits() map[string][]string {
	return b.details.AssignedUnits
}

// Config returns the configuration changes that apply to the branch.
func (b *Branch) Config() map[string]settings.ItemChanges {
	return b.details.Config
}

func (b *Branch) setDetails(details BranchChange) {
	// If this is the first receipt of details, set the removal message.
	if b.removalMessage == nil {
		b.removalMessage = RemoveBranch{
			ModelUUID: details.ModelUUID,
			Id:        details.Id,
		}
	}

	b.setStale(false)

	b.details = details
	b.hub.Publish(branchChange, b.copy())
}

// copy returns a copy of the branch, ensuring appropriate deep copying.
func (b *Branch) copy() Branch {
	cb := *b
	cb.details = cb.details.copy()
	return cb
}
