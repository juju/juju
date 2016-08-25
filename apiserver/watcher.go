// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterFacade(
		"AllWatcher", 1, NewAllWatcher,
		reflect.TypeOf((*SrvAllWatcher)(nil)),
	)
	// Note: AllModelWatcher uses the same infrastructure as AllWatcher
	// but they are get under separate names as it possible the may
	// diverge in the future (especially in terms of authorisation
	// checks).
	common.RegisterFacade(
		"AllModelWatcher", 2, NewAllWatcher,
		reflect.TypeOf((*SrvAllWatcher)(nil)),
	)
	common.RegisterFacade(
		"NotifyWatcher", 1, newNotifyWatcher,
		reflect.TypeOf((*srvNotifyWatcher)(nil)),
	)
	common.RegisterFacade(
		"StringsWatcher", 1, newStringsWatcher,
		reflect.TypeOf((*srvStringsWatcher)(nil)),
	)
	common.RegisterFacade(
		"RelationUnitsWatcher", 1, newRelationUnitsWatcher,
		reflect.TypeOf((*srvRelationUnitsWatcher)(nil)),
	)
	common.RegisterFacade(
		"VolumeAttachmentsWatcher", 2, newVolumeAttachmentsWatcher,
		reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)),
	)
	common.RegisterFacade(
		"FilesystemAttachmentsWatcher", 2, newFilesystemAttachmentsWatcher,
		reflect.TypeOf((*srvMachineStorageIdsWatcher)(nil)),
	)
	common.RegisterFacade(
		"EntityWatcher", 2, newEntitiesWatcher,
		reflect.TypeOf((*srvEntitiesWatcher)(nil)),
	)
	common.RegisterFacade(
		"MigrationStatusWatcher", 1, newMigrationStatusWatcher,
		reflect.TypeOf((*srvMigrationStatusWatcher)(nil)),
	)
}

// NewAllWatcher returns a new API server endpoint for interacting
// with a watcher created by the WatchAll and WatchAllModels API calls.
func NewAllWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	// HasPermission should not be replaced by auth.AuthClient() even if, at first sight, they seem
	// equivalent because this allows us to remove login permission for a user
	// (a permission that is given by default).
	isAuthorized, err := auth.HasPermission(description.LoginAccess, context.State().ControllerTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isAuthorized {
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
// the AllWatcher and AllModelWatcher facades.
type SrvAllWatcher struct {
	watcher   *state.Multiwatcher
	id        string
	resources facade.Resources
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
	resources facade.Resources
}

func isAgent(auth facade.Authorizer) bool {
	return auth.AuthMachineAgent() || auth.AuthUnitAgent()
}

func newNotifyWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

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
	resources facade.Resources
}

func newStringsWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

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
	resources facade.Resources
}

func newRelationUnitsWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

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
	resources facade.Resources
	parser    func([]string) ([]params.MachineStorageId, error)
}

func newVolumeAttachmentsWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()
	st := context.State()

	return newMachineStorageIdsWatcher(
		st, resources, auth, id, storagecommon.ParseVolumeAttachmentIds,
	)
}

func newFilesystemAttachmentsWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()
	st := context.State()
	return newMachineStorageIdsWatcher(
		st, resources, auth, id, storagecommon.ParseFilesystemAttachmentIds,
	)
}

func newMachineStorageIdsWatcher(
	st *state.State,
	resources facade.Resources,
	auth facade.Authorizer,
	id string,
	parser func([]string) ([]params.MachineStorageId, error),
) (facade.Facade, error) {
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

// EntitiesWatcher defines an interface based on the StringsWatcher
// but also providing a method for the mapping of the received
// strings to the tags of the according entities.
type EntitiesWatcher interface {
	state.StringsWatcher

	// MapChanges maps the received strings to their according tag strings.
	// The EntityFinder interface representing state or a mock has to be
	// upcasted into the needed sub-interface of state for the real mapping.
	MapChanges(in []string) ([]string, error)
}

// srvEntitiesWatcher defines the API for methods on a state.StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvEntitiesWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings, which could be transformed
// from state entity ids to their corresponding entity tags.
type srvEntitiesWatcher struct {
	resources facade.Resources
	id        string
	watcher   EntitiesWatcher
}

func newEntitiesWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	if !isAgent(auth) {
		return nil, common.ErrPerm
	}
	watcher, ok := resources.Get(id).(EntitiesWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvEntitiesWatcher{
		resources: resources,
		id:        id,
		watcher:   watcher,
	}, nil
}

// Next returns when a change has occured to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvEntitiesWatcher.
func (w *srvEntitiesWatcher) Next() (params.EntitiesWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		mapped, err := w.watcher.MapChanges(changes)
		if err != nil {
			return params.EntitiesWatchResult{}, errors.Annotate(err, "cannot map changes")
		}
		return params.EntitiesWatchResult{
			Changes: mapped,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.EntitiesWatchResult{}, err
}

// Stop stops the watcher.
func (w *srvEntitiesWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

var getMigrationBackend = func(st *state.State) migrationBackend {
	return st
}

// migrationBackend defines State functionality required by the
// migration watchers.
type migrationBackend interface {
	LatestMigration() (state.ModelMigration, error)
	APIHostPorts() ([][]network.HostPort, error)
	ControllerConfig() (controller.Config, error)
}

func newMigrationStatusWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()
	st := context.State()

	if !(auth.AuthMachineAgent() || auth.AuthUnitAgent()) {
		return nil, common.ErrPerm
	}
	w, ok := resources.Get(id).(state.NotifyWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvMigrationStatusWatcher{
		watcher:   w,
		id:        id,
		resources: resources,
		st:        getMigrationBackend(st),
	}, nil
}

type srvMigrationStatusWatcher struct {
	watcher   state.NotifyWatcher
	id        string
	resources facade.Resources
	st        migrationBackend
}

// Next returns when the status for a model migration for the
// associated model changes. The current details for the active
// migration are returned.
func (w *srvMigrationStatusWatcher) Next() (params.MigrationStatus, error) {
	empty := params.MigrationStatus{}

	if _, ok := <-w.watcher.Changes(); !ok {
		err := w.watcher.Err()
		if err == nil {
			err = common.ErrStoppedWatcher
		}
		return empty, err
	}

	mig, err := w.st.LatestMigration()
	if errors.IsNotFound(err) {
		return params.MigrationStatus{
			Phase: migration.NONE.String(),
		}, nil
	} else if err != nil {
		return empty, errors.Annotate(err, "migration lookup")
	}

	attempt, err := mig.Attempt()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving migration attempt")
	}

	phase, err := mig.Phase()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving migration phase")
	}

	sourceAddrs, err := w.getLocalHostPorts()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving source addresses")
	}

	sourceCACert, err := getControllerCACert(w.st)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving source CA cert")
	}

	target, err := mig.TargetInfo()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving target info")
	}

	return params.MigrationStatus{
		MigrationId:    mig.Id(),
		Attempt:        attempt,
		Phase:          phase.String(),
		SourceAPIAddrs: sourceAddrs,
		SourceCACert:   sourceCACert,
		TargetAPIAddrs: target.Addrs,
		TargetCACert:   target.CACert,
	}, nil
}

func (w *srvMigrationStatusWatcher) getLocalHostPorts() ([]string, error) {
	hostports, err := w.st.APIHostPorts()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var out []string
	for _, section := range hostports {
		for _, hostport := range section {
			out = append(out, hostport.String())
		}
	}
	return out, nil
}

// Stop stops the watcher.
func (w *srvMigrationStatusWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// This is a shim to avoid the need to use a working State into the
// unit tests. It is tested as part of the client side API tests.
var getControllerCACert = func(st migrationBackend) (string, error) {
	cfg, err := st.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}

	cacert, ok := cfg.CACert()
	if !ok {
		return "", errors.New("missing CA cert for controller model")
	}
	return cacert, nil
}
