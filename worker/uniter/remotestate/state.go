// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"time"

	"github.com/juju/names/v4"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type Waiter interface {
	After() <-chan time.Time
}

type UpdateStatusTimerFunc func(time.Duration) Waiter

type State interface {
	Charm(string) (Charm, error)
	Relation(names.RelationTag) (Relation, error)
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	StorageAttachmentLife([]params.StorageAttachmentId) ([]params.LifeResult, error)
	Unit(names.UnitTag) (Unit, error)
	WatchRelationUnits(names.RelationTag, names.UnitTag) (watcher.RelationUnitsWatcher, error)
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error)
	UpdateStatusHookInterval() (time.Duration, error)
}

type Unit interface {
	Life() life.Value
	LXDProfileName() (string, error)
	Refresh() error
	ProviderID() string
	Resolved() params.ResolvedMode
	Application() (Application, error)
	Tag() names.UnitTag
	Watch() (watcher.NotifyWatcher, error)
	WatchAddressesHash() (watcher.StringsWatcher, error)
	WatchConfigSettingsHash() (watcher.StringsWatcher, error)
	WatchTrustConfigSettingsHash() (watcher.StringsWatcher, error)
	WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error)
	WatchInstanceData() (watcher.NotifyWatcher, error)
	WatchStorage() (watcher.StringsWatcher, error)
	WatchActionNotifications() (watcher.StringsWatcher, error)
	// WatchRelations returns a watcher that fires when relations
	// relevant for this unit change.
	WatchRelations() (watcher.StringsWatcher, error)
	UpgradeSeriesStatus() (model.UpgradeSeriesStatus, string, error)
}

type Application interface {
	// CharmModifiedVersion returns a revision number for the charm that
	// increments whenever the charm or a resource for the charm changes.
	CharmModifiedVersion() (int, error)
	// CharmURL returns the url for the charm for this application.
	CharmURL() (string, bool, error)
	// Life returns whether the application is alive.
	Life() life.Value
	// Refresh syncs this value with the api server.
	Refresh() error
	// Tag returns the tag for this application.
	Tag() names.ApplicationTag
	// Watch returns a watcher that fires when this application changes.
	Watch() (watcher.NotifyWatcher, error)
	// WatchLeadershipSettings returns a watcher that fires when the leadership
	// settings for this application change.
	WatchLeadershipSettings() (watcher.NotifyWatcher, error)
}

type Relation interface {
	Id() int
	Tag() names.RelationTag
	Life() life.Value
	Suspended() bool
	UpdateSuspended(bool)
}

type Charm interface {
	// LXDProfileRequired returns true if this charm has an lxdprofile.yaml
	LXDProfileRequired() (bool, error)
}

func NewAPIState(st *uniter.State) State {
	return apiState{st}
}

type apiState struct {
	*uniter.State
}

type apiUnit struct {
	*uniter.Unit
}

type apiApplication struct {
	*uniter.Application
}

type apiRelation struct {
	*uniter.Relation
}

func (st apiState) Relation(tag names.RelationTag) (Relation, error) {
	r, err := st.State.Relation(tag)
	return apiRelation{r}, err
}

func (st apiState) Unit(tag names.UnitTag) (Unit, error) {
	u, err := st.State.Unit(tag)
	return apiUnit{u}, err
}

func (st apiState) Charm(charmURL string) (Charm, error) {
	return st.State.Charm(charmURL)
}

func (u apiUnit) Application() (Application, error) {
	s, err := u.Unit.Application()
	return apiApplication{s}, err
}
