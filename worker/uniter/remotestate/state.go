// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

type State interface {
	Relation(names.RelationTag) (Relation, error)
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	StorageAttachmentLife([]params.StorageAttachmentId) ([]params.LifeResult, error)
	Unit(names.UnitTag) (Unit, error)
	WatchRelationUnits(names.RelationTag, names.UnitTag) (watcher.RelationUnitsWatcher, error)
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
}

type Unit interface {
	Life() params.Life
	Refresh() error
	Resolved() (params.ResolvedMode, error)
	Service() (Service, error)
	Tag() names.UnitTag
	Watch() (watcher.NotifyWatcher, error)
	WatchAddresses() (watcher.NotifyWatcher, error)
	WatchConfigSettings() (watcher.NotifyWatcher, error)
	WatchStorage() (watcher.StringsWatcher, error)
	WatchActionNotifications() (watcher.StringsWatcher, error)
}

type Service interface {
	// CharmModifiedVersion returns a revision number for the charm that
	// increments whenever the charm or a resource for the charm changes.
	CharmModifiedVersion() (int, error)
	// CharmURL returns the url for the charm for this service.
	CharmURL() (*charm.URL, bool, error)
	// Life returns whether the service is alive.
	Life() params.Life
	// Refresh syncs this value with the api server.
	Refresh() error
	// Tag returns the tag for this service.
	Tag() names.ServiceTag
	// Watch returns a watcher that fires when this service changes.
	Watch() (watcher.NotifyWatcher, error)
	// WatchLeadershipSettings returns a watcher that fires when the leadership
	// settings for this service change.
	WatchLeadershipSettings() (watcher.NotifyWatcher, error)
	// WatchRelation returns a watcher that fires when the relations on this
	// service change.
	WatchRelations() (watcher.StringsWatcher, error)
}

type Relation interface {
	Id() int
	Life() params.Life
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

type apiService struct {
	*uniter.Service
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

func (u apiUnit) Service() (Service, error) {
	s, err := u.Unit.Service()
	return apiService{s}, err
}
