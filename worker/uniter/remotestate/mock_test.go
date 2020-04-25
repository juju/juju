// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"sync"
	"time"

	"github.com/juju/charm/v7"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/names/v4"
)

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		stopped: make(chan struct{}),
	}
}

type mockWatcher struct {
	mu      sync.Mutex
	stopped chan struct{}
}

func (w *mockWatcher) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.Stopped() {
		close(w.stopped)
	}
}

func (w *mockWatcher) Wait() error {
	<-w.stopped
	return nil
}

func (w *mockWatcher) Stopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	return &mockNotifyWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan struct{}, 1),
	}
}

type mockNotifyWatcher struct {
	*mockWatcher
	changes chan struct{}
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

func newMockStringsWatcher() *mockStringsWatcher {
	return &mockStringsWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan []string, 1),
	}
}

type mockStringsWatcher struct {
	*mockWatcher
	changes chan []string
}

func (w *mockStringsWatcher) Changes() watcher.StringsChannel {
	return w.changes
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	return &mockRelationUnitsWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan watcher.RelationUnitsChange, 1),
	}
}

type mockRelationUnitsWatcher struct {
	*mockWatcher
	changes chan watcher.RelationUnitsChange
}

func (w *mockRelationUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	return w.changes
}

type mockState struct {
	modelType                   model.ModelType
	unit                        mockUnit
	relations                   map[names.RelationTag]*mockRelation
	storageAttachment           map[params.StorageAttachmentId]params.StorageAttachment
	relationUnitsWatchers       map[names.RelationTag]*mockRelationUnitsWatcher
	relationAppWatchers         map[names.RelationTag]map[string]*mockNotifyWatcher
	storageAttachmentWatchers   map[names.StorageTag]*mockNotifyWatcher
	updateStatusInterval        time.Duration
	updateStatusIntervalWatcher *mockNotifyWatcher
}

func (st *mockState) Relation(tag names.RelationTag) (remotestate.Relation, error) {
	r, ok := st.relations[tag]
	if !ok {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return r, nil
}

func (st *mockState) StorageAttachment(
	storageTag names.StorageTag, unitTag names.UnitTag,
) (params.StorageAttachment, error) {
	if unitTag != st.unit.tag {
		return params.StorageAttachment{}, &params.Error{Code: params.CodeNotFound}
	}
	attachment, ok := st.storageAttachment[params.StorageAttachmentId{
		UnitTag:    unitTag.String(),
		StorageTag: storageTag.String(),
	}]
	if !ok {
		return params.StorageAttachment{}, &params.Error{Code: params.CodeNotFound}
	}
	if attachment.Kind == params.StorageKindUnknown {
		return params.StorageAttachment{}, &params.Error{Code: params.CodeNotProvisioned}
	}
	return attachment, nil
}

func (st *mockState) StorageAttachmentLife(
	ids []params.StorageAttachmentId,
) ([]params.LifeResult, error) {
	results := make([]params.LifeResult, len(ids))
	for i, id := range ids {
		attachment, ok := st.storageAttachment[id]
		if !ok {
			results[i] = params.LifeResult{
				Error: &params.Error{Code: params.CodeNotFound},
			}
			continue
		}
		results[i] = params.LifeResult{Life: attachment.Life}
	}
	return results, nil
}

func (st *mockState) Unit(tag names.UnitTag) (remotestate.Unit, error) {
	if tag != st.unit.tag {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return &st.unit, nil
}

func (st *mockState) WatchRelationUnits(
	relationTag names.RelationTag, unitTag names.UnitTag,
) (watcher.RelationUnitsWatcher, error) {
	if unitTag != st.unit.tag {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	watcher, ok := st.relationUnitsWatchers[relationTag]
	if !ok {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return watcher, nil
}

func (st *mockState) WatchStorageAttachment(
	storageTag names.StorageTag, unitTag names.UnitTag,
) (watcher.NotifyWatcher, error) {
	if unitTag != st.unit.tag {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	watcher, ok := st.storageAttachmentWatchers[storageTag]
	if !ok {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return watcher, nil
}

func (st *mockState) UpdateStatusHookInterval() (time.Duration, error) {
	return st.updateStatusInterval, nil
}

func (st *mockState) WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error) {
	return st.updateStatusIntervalWatcher, nil
}

type mockUnit struct {
	tag                              names.UnitTag
	life                             life.Value
	providerID                       string
	resolved                         params.ResolvedMode
	application                      mockApplication
	unitWatcher                      *mockNotifyWatcher
	addressesWatcher                 *mockStringsWatcher
	configSettingsWatcher            *mockStringsWatcher
	applicationConfigSettingsWatcher *mockStringsWatcher
	upgradeSeriesWatcher             *mockNotifyWatcher
	storageWatcher                   *mockStringsWatcher
	actionWatcher                    *mockStringsWatcher
	relationsWatcher                 *mockStringsWatcher
}

func (u *mockUnit) Life() life.Value {
	return u.life
}

func (u *mockUnit) Refresh() error {
	return nil
}

func (u *mockUnit) ProviderID() string {
	return u.providerID
}

func (u *mockUnit) Resolved() params.ResolvedMode {
	return u.resolved
}

func (u *mockUnit) Application() (remotestate.Application, error) {
	return &u.application, nil
}

func (u *mockUnit) Tag() names.UnitTag {
	return u.tag
}

func (u *mockUnit) Watch() (watcher.NotifyWatcher, error) {
	return u.unitWatcher, nil
}

func (u *mockUnit) WatchAddressesHash() (watcher.StringsWatcher, error) {
	return u.addressesWatcher, nil
}

func (u *mockUnit) WatchConfigSettingsHash() (watcher.StringsWatcher, error) {
	return u.configSettingsWatcher, nil
}

func (u *mockUnit) WatchTrustConfigSettingsHash() (watcher.StringsWatcher, error) {
	return u.applicationConfigSettingsWatcher, nil
}

func (u *mockUnit) WatchStorage() (watcher.StringsWatcher, error) {
	return u.storageWatcher, nil
}

func (u *mockUnit) WatchActionNotifications() (watcher.StringsWatcher, error) {
	return u.actionWatcher, nil
}

func (u *mockUnit) WatchRelations() (watcher.StringsWatcher, error) {
	return u.relationsWatcher, nil
}

func (u *mockUnit) WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error) {
	return u.upgradeSeriesWatcher, nil
}

func (u *mockUnit) UpgradeSeriesStatus() (model.UpgradeSeriesStatus, error) {
	return model.UpgradeSeriesPrepareStarted, nil
}

func (u *mockUnit) SetUpgradeSeriesStatus(status model.UpgradeSeriesStatus) error {
	return nil
}

type mockApplication struct {
	tag                   names.ApplicationTag
	life                  life.Value
	curl                  *charm.URL
	charmModifiedVersion  int
	forceUpgrade          bool
	applicationWatcher    *mockNotifyWatcher
	leaderSettingsWatcher *mockNotifyWatcher
}

func (s *mockApplication) CharmModifiedVersion() (int, error) {
	return s.charmModifiedVersion, nil
}

func (s *mockApplication) CharmURL() (*charm.URL, bool, error) {
	return s.curl, s.forceUpgrade, nil
}

func (s *mockApplication) Life() life.Value {
	return s.life
}

func (s *mockApplication) Refresh() error {
	return nil
}

func (s *mockApplication) Tag() names.ApplicationTag {
	return s.tag
}

func (s *mockApplication) Watch() (watcher.NotifyWatcher, error) {
	return s.applicationWatcher, nil
}

func (s *mockApplication) WatchLeadershipSettings() (watcher.NotifyWatcher, error) {
	return s.leaderSettingsWatcher, nil
}

type mockRelation struct {
	tag       names.RelationTag
	id        int
	life      life.Value
	suspended bool
}

func (r *mockRelation) Tag() names.RelationTag {
	return r.tag
}

func (r *mockRelation) Id() int {
	return r.id
}

func (r *mockRelation) Life() life.Value {
	return r.life
}

func (r *mockRelation) Suspended() bool {
	return r.suspended
}

func (r *mockRelation) UpdateSuspended(suspended bool) {
	r.suspended = suspended
}

type mockLeadershipTracker struct {
	leadership.Tracker
	claimTicket  mockTicket
	leaderTicket mockTicket
	minionTicket mockTicket
}

func (mock *mockLeadershipTracker) ClaimLeader() leadership.Ticket {
	return &mock.claimTicket
}

func (mock *mockLeadershipTracker) WaitLeader() leadership.Ticket {
	return &mock.leaderTicket
}

func (mock *mockLeadershipTracker) WaitMinion() leadership.Ticket {
	return &mock.minionTicket
}

type mockTicket struct {
	ch     chan struct{}
	result bool
}

func (t *mockTicket) Ready() <-chan struct{} {
	return t.ch
}

func (t *mockTicket) Wait() bool {
	return t.result
}
