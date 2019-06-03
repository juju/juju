// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"

	"github.com/juju/juju/core/settings"
)

// Branch represents an active branch in a cached model.
type Branch struct {
	// Resident identifies the branch as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

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

// AssignedUnits returns a map of the names of units tracking this branch,
// keyed by application names with changes made under the branch.
func (b *Branch) AssignedUnits() map[string][]string {
	return b.details.AssignedUnits
}

func (b *Branch) setDetails(details BranchChange) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If this is the first receipt of details, set the removal message.
	if b.removalMessage == nil {
		b.removalMessage = RemoveBranch{
			ModelUUID: details.ModelUUID,
			Id:        details.Id,
		}
	}

	b.setStale(false)

	// TODO (manadart 2019-05-29): Publish changes for config deltas and tracking units.

	b.details = details
}

// copy returns a copy of the branch, ensuring appropriate deep copying.
// This method is called while the cache model is locked,
// and so should no require its own lock protection.
func (b *Branch) copy() Branch {
	var cAssignedUnits map[string][]string
	bAssignedUnits := b.details.AssignedUnits
	if bAssignedUnits != nil {
		cAssignedUnits = make(map[string][]string, len(bAssignedUnits))
		for k, v := range bAssignedUnits {
			units := make([]string, len(v))
			for i, u := range v {
				units[i] = u
			}
			cAssignedUnits[k] = units
		}
	}

	var cConfig map[string]settings.ItemChanges
	bConfig := b.details.Config
	if bConfig != nil {
		cConfig = make(map[string]settings.ItemChanges, len(bConfig))
		for k, v := range bConfig {
			changes := make(settings.ItemChanges, len(v))
			for i, ch := range v {
				changes[i] = settings.ItemChange{
					Type:     ch.Type,
					Key:      ch.Key,
					NewValue: ch.NewValue,
					OldValue: ch.OldValue,
				}
			}
			cConfig[k] = changes
		}
	}

	cb := *b
	cb.details.AssignedUnits = cAssignedUnits
	cb.details.Config = cConfig

	return cb
}
