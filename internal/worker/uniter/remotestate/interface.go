// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/rpc/params"
)

// The interfaces in this file provide access to the various facade api methods.

type Watcher interface {
	// RemoteStateChanged returns a channel which is signalled
	// whenever the remote state is changed.
	RemoteStateChanged() <-chan struct{}

	// Snapshot returns the current snapshot of the remote state.
	Snapshot() Snapshot

	worker.Worker
}

type Waiter interface {
	After() <-chan time.Time
}

type UpdateStatusTimerFunc func(duration time.Duration) Waiter

type UniterClient interface {
	Charm(url string) (api.Charm, error)
	Relation(tag names.RelationTag) (api.Relation, error)
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	StorageAttachmentLife([]params.StorageAttachmentId) ([]params.LifeResult, error)
	Unit(names.UnitTag) (api.Unit, error)
	WatchRelationUnits(names.RelationTag, names.UnitTag) (watcher.RelationUnitsWatcher, error)
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error)
	UpdateStatusHookInterval() (time.Duration, error)
}

type Charm interface {
	// LXDProfileRequired returns true if this charm has an lxdprofile.yaml
	LXDProfileRequired() (bool, error)
}
