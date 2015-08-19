// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterFacade(
		"AllWatcher", 0, NewAllWatcher,
		reflect.TypeOf((*SrvAllWatcher)(nil)),
	)
	// Note: AllEnvWatcher uses the same infrastructure as AllWatcher
	// but they are get under separate names as it possible the may
	// diverge in the future (especially in terms of authorisation
	// checks).
	common.RegisterFacade(
		"AllEnvWatcher", 1, NewAllWatcher,
		reflect.TypeOf((*SrvAllWatcher)(nil)),
	)
	common.RegisterFacade(
		"NotifyWatcher", 0, newNotifyWatcher,
		reflect.TypeOf((*srvNotifyWatcher)(nil)),
	)
	common.RegisterFacade(
		"StringsWatcher", 0, newStringsWatcher,
		reflect.TypeOf((*srvStringsWatcher)(nil)),
	)
	common.RegisterFacade(
		"RelationUnitsWatcher", 0, newRelationUnitsWatcher,
		reflect.TypeOf((*srvRelationUnitsWatcher)(nil)),
	)
	common.RegisterFacade(
		"VolumeAttachmentsWatcher", 1, newVolumeAttachmentsWatcher,
		reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)),
	)
	common.RegisterFacade(
		"FilesystemAttachmentsWatcher", 1, newFilesystemAttachmentsWatcher,
		reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)),
	)
	common.RegisterFacade(
		"EntityWatcher", 1, newEntityWatcher,
		reflect.TypeOf((*srvEntityWatcher)(nil)),
	)
}

// NewAllEnvWatcher returns a new API server endpoint for interacting
// with a watcher created by the WatchAll and WatchAllEnvs API calls.
func NewAllWatcher(st *state.State, resources *common.Resources, auth common.Authorizer, id string) (interface{}, error) {
	if !auth.AuthClient() {
		return nil, common.ErrPerm
	}

	watcher, ok := resources.Get(id).(*state.Multiwatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &SrvAllWatcher{
		watcher:   watcher,
		id:        id,
		resources: resources,
	}, nil
}

// SrvAllWatcher defines the API methods on a state.Multiwatcher.
// which watches any changes to the state. Each client has its own
// current set of watchers, stored in resources. It is used by both
// the AllWatcher and AllEnvWatcher facades.
type SrvAllWatcher struct {
	watcher   *state.Multiwatcher
	id        string
	resources *common.Resources
}

func (aw *SrvAllWatcher) Next() (params.AllWatcherNextResults, error) {
	deltas, err := aw.watcher.Next()
	return params.AllWatcherNextResults{
		Deltas: deltas,
	}, err
}

func (w *SrvAllWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// srvNotifyWatcher defines the API access to methods on a state.NotifyWatcher.
// Each client has its own current set of watchers, stored in resources.
type srvNotifyWatcher struct {
	watcher   state.NotifyWatcher
	id        string
	resources *common.Resources
}

func isAgent(auth common.Authorizer) bool {
	return auth.AuthMachineAgent() || auth.AuthUnitAgent()
}

func newNotifyWatcher(st *state.State, resources *common.Resources, auth common.Authorizer, id string) (interface{}, error) {
	if !isAgent(auth) {
		return nil, common.ErrPerm
	}
	watcher, ok := resources.Get(id).(state.NotifyWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvNotifyWatcher{
		watcher:   watcher,
		id:        id,
		resources: resources,
	}, nil
}

// Next returns when a change has occurred to the
// entity being watched since the most recent call to Next
// or the Watch call that created the NotifyWatcher.
func (w *srvNotifyWatcher) Next() error {
	if _, ok := <-w.watcher.Changes(); ok {
		return nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return err
}

// Stop stops the watcher.
func (w *srvNotifyWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// srvStringsWatcher defines the API for methods on a state.StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvStringsWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings.
type srvStringsWatcher struct {
	watcher   state.StringsWatcher
	id        string
	resources *common.Resources
}

func newStringsWatcher(st *state.State, resources *common.Resources, auth common.Authorizer, id string) (interface{}, error) {
	if !isAgent(auth) {
		return nil, common.ErrPerm
	}
	watcher, ok := resources.Get(id).(state.StringsWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvStringsWatcher{
		watcher:   watcher,
		id:        id,
		resources: resources,
	}, nil
}

// Next returns when a change has occured to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvStringsWatcher.
func (w *srvStringsWatcher) Next() (params.StringsWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.StringsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.StringsWatchResult{}, err
}

// Stop stops the watcher.
func (w *srvStringsWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// srvRelationUnitsWatcher defines the API wrapping a state.RelationUnitsWatcher.
// It notifies about units entering and leaving the scope of a RelationUnit,
// and changes to the settings of those units known to have entered.
type srvRelationUnitsWatcher struct {
	watcher   state.RelationUnitsWatcher
	id        string
	resources *common.Resources
}

func newRelationUnitsWatcher(st *state.State, resources *common.Resources, auth common.Authorizer, id string) (interface{}, error) {
	if !isAgent(auth) {
		return nil, common.ErrPerm
	}
	watcher, ok := resources.Get(id).(state.RelationUnitsWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvRelationUnitsWatcher{
		watcher:   watcher,
		id:        id,
		resources: resources,
	}, nil
}

// Next returns when a change has occured to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationUnitsWatcher.
func (w *srvRelationUnitsWatcher) Next() (params.RelationUnitsWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.RelationUnitsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.RelationUnitsWatchResult{}, err
}

// Stop stops the watcher.
func (w *srvRelationUnitsWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// srvMachineStorageIdsWatcher defines the API wrapping a state.StringsWatcher
// watching machine/storage attachments. This watcher notifies about storage
// entities (volumes/filesystems) being attached to and detached from machines.
//
// TODO(axw) state needs a new watcher, this is a bt of a hack. State watchers
// could do with some deduplication of logic, and I don't want to add to that
// spaghetti right now.
type srvMachineStorageIdsWatcher struct {
	watcher   state.StringsWatcher
	id        string
	resources *common.Resources
	parser    func([]string) ([]params.MachineStorageId, error)
}

func newVolumeAttachmentsWatcher(
	st *state.State,
	resources *common.Resources,
	auth common.Authorizer,
	id string,
) (interface{}, error) {
	return newMachineStorageIdsWatcher(
		st, resources, auth, id, common.ParseVolumeAttachmentIds,
	)
}

func newFilesystemAttachmentsWatcher(
	st *state.State,
	resources *common.Resources,
	auth common.Authorizer,
	id string,
) (interface{}, error) {
	return newMachineStorageIdsWatcher(
		st, resources, auth, id, common.ParseFilesystemAttachmentIds,
	)
}

func newMachineStorageIdsWatcher(
	st *state.State,
	resources *common.Resources,
	auth common.Authorizer,
	id string,
	parser func([]string) ([]params.MachineStorageId, error),
) (interface{}, error) {
	if !isAgent(auth) {
		return nil, common.ErrPerm
	}
	watcher, ok := resources.Get(id).(state.StringsWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvMachineStorageIdsWatcher{watcher, id, resources, parser}, nil
}

// Next returns when a change has occured to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvMachineStorageIdsWatcher.
func (w *srvMachineStorageIdsWatcher) Next() (params.MachineStorageIdsWatchResult, error) {
	if stringChanges, ok := <-w.watcher.Changes(); ok {
		changes, err := w.parser(stringChanges)
		if err != nil {
			return params.MachineStorageIdsWatchResult{}, err
		}
		return params.MachineStorageIdsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.MachineStorageIdsWatchResult{}, err
}

// Stop stops the watcher.
func (w *srvMachineStorageIdsWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// EntityWatcher defines an interface based on the StringsWatcher
// but also providing a method for the mapping of the received
// strings to the tags of the according entities.
type EntityWatcher interface {
	state.StringsWatcher

	// MapChanges maps the received strings to their according tag strings.
	// The EntityFinder interface representing state or a mock has to be
	// upcasted into the needed sub-interface of state for the real mapping.
	MapChanges(in []string) ([]string, error)
}

// srvEntityWatcher defines the API for methods on a state.StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvEntityWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings, which could be transformed
// from state entity ids to their corresponding entity tags.
type srvEntityWatcher struct {
	st        *state.State
	resources *common.Resources
	id        string
	watcher   EntityWatcher
}

func newEntityWatcher(st *state.State, resources *common.Resources, auth common.Authorizer, id string) (interface{}, error) {
	if !isAgent(auth) {
		return nil, common.ErrPerm
	}
	watcher, ok := resources.Get(id).(EntityWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvEntityWatcher{
		st:        st,
		resources: resources,
		id:        id,
		watcher:   watcher,
	}, nil
}

// Next returns when a change has occured to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvEntityWatcher.
func (w *srvEntityWatcher) Next() (params.EntityWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		mapped, err := w.watcher.MapChanges(changes)
		if err != nil {
			return params.EntityWatchResult{}, errors.Annotate(err, "cannot map changes")
		}
		return params.EntityWatchResult{
			Changes: mapped,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.EntityWatchResult{}, err
}

// Stop stops the watcher.
func (w *srvEntityWatcher) Stop() error {
	return w.resources.Stop(w.id)
}
