// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type watcherCommon struct {
	id              string
	resources       facade.Resources
	watcherRegistry facade.WatcherRegistry
	dispose         func()
}

func newWatcherCommon(context facade.Context) watcherCommon {
	return watcherCommon{
		id:              context.ID(),
		resources:       context.Resources(),
		watcherRegistry: context.WatcherRegistry(),
		dispose:         context.Dispose,
	}
}

// Stop stops the watcher.
func (w *watcherCommon) Stop() error {
	w.dispose()
	if _, err := w.watcherRegistry.Get(w.id); err == nil {
		return errors.Trace(w.watcherRegistry.Stop(w.id))
	}
	return errors.Trace(w.resources.Stop(w.id))
}

// GetWatcherByID returns the watcher with the given ID.
// Deprecated: This only exists to support the old watcher API, once resources
// have been removed, this can be removed too.
func GetWatcherByID(watcherRegistry facade.WatcherRegistry, resources facade.Resources, id string) (worker.Worker, error) {
	watcher, err := watcherRegistry.Get(id)
	if err == nil {
		return watcher, nil
	}
	return resources.Get(id), nil
}

func isAgent(auth facade.Authorizer) bool {
	return auth.AuthMachineAgent() || auth.AuthUnitAgent() || auth.AuthApplicationAgent() || auth.AuthModelAgent()
}

func isAgentOrUser(auth facade.Authorizer) bool {
	return isAgent(auth) || auth.AuthClient()
}

func newNotifyWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgentOrUser(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.NotifyWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}

	return &srvNotifyWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// srvNotifyWatcher defines the API access to methods on a NotifyWatcher.
// Each client has its own current set of watchers, stored in resources.
type srvNotifyWatcher struct {
	watcherCommon
	watcher corewatcher.NotifyWatcher
}

// Next returns when a change has occurred to the
// entity being watched since the most recent call to Next
// or the Watch call that created the NotifyWatcher.
func (w *srvNotifyWatcher) Next(ctx context.Context) error {
	_, err := internal.FirstResult[struct{}](ctx, w.watcher)
	return errors.Trace(err)
}

// srvStringsWatcher defines the API for methods on a StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvStringsWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings.
type srvStringsWatcher struct {
	watcherCommon
	watcher corewatcher.StringsWatcher
}

func newStringsWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgentOrUser(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvStringsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvStringsWatcher.
func (w *srvStringsWatcher) Next(ctx context.Context) (params.StringsWatchResult, error) {
	changes, err := internal.FirstResult[[]string](ctx, w.watcher)
	if err != nil {
		return params.StringsWatchResult{}, errors.Trace(err)
	}
	return params.StringsWatchResult{
		Changes: changes,
	}, nil
}

// srvRelationUnitsWatcher defines the API wrapping a RelationUnitsWatcher.
// It notifies about units entering and leaving the scope of a RelationUnit,
// and changes to the settings of those units known to have entered.
type srvRelationUnitsWatcher struct {
	watcherCommon
	watcher common.RelationUnitsWatcher
}

func newRelationUnitsWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(common.RelationUnitsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRelationUnitsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationUnitsWatcher.
func (w *srvRelationUnitsWatcher) Next(ctx context.Context) (params.RelationUnitsWatchResult, error) {
	changes, err := internal.FirstResult[params.RelationUnitsChange](ctx, w.watcher)
	if err != nil {
		return params.RelationUnitsWatchResult{}, errors.Trace(err)
	}
	return params.RelationUnitsWatchResult{
		Changes: changes,
	}, nil
}

// srvRemoteRelationWatcher defines the API wrapping a
// RelationUnitsWatcher but serving the events it emits as
// fully-expanded params.RemoteRelationChangeEvents so they can be
// used across model/controller boundaries.
type srvRemoteRelationWatcher struct {
	watcherCommon
	backend crossmodel.Backend
	watcher *crossmodel.WrappedUnitsWatcher
}

func newRemoteRelationWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(*crossmodel.WrappedUnitsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRemoteRelationWatcher{
		watcherCommon: newWatcherCommon(context),
		backend:       crossmodel.GetBackend(context.State()),
		watcher:       watcher,
	}, nil
}

func (w *srvRemoteRelationWatcher) Next(ctx context.Context) (params.RemoteRelationWatchResult, error) {
	changes, err := internal.FirstResult[params.RelationUnitsChange](ctx, w.watcher)
	if err != nil {
		return params.RemoteRelationWatchResult{}, errors.Trace(err)
	}
	// Expand the change into a cross-model event.
	expanded, err := crossmodel.ExpandChange(
		w.backend,
		w.watcher.RelationToken,
		w.watcher.ApplicationOrOfferToken,
		changes,
	)
	if err != nil {
		return params.RemoteRelationWatchResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}
	return params.RemoteRelationWatchResult{
		Changes: expanded,
	}, nil
}

// srvRelationStatusWatcher defines the API wrapping a RelationStatusWatcher.
type srvRelationStatusWatcher struct {
	watcherCommon
	st      *state.State
	watcher corewatcher.StringsWatcher
}

func newRelationStatusWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRelationStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationStatusWatcher.
func (w *srvRelationStatusWatcher) Next(ctx context.Context) (params.RelationLifeSuspendedStatusWatchResult, error) {
	changes, err := internal.FirstResult[[]string](ctx, w.watcher)
	if err != nil {
		return params.RelationLifeSuspendedStatusWatchResult{}, errors.Trace(err)
	}
	changesParams := make([]params.RelationLifeSuspendedStatusChange, len(changes))
	for i, key := range changes {
		change, err := crossmodel.GetRelationLifeSuspendedStatusChange(crossmodel.GetBackend(w.st), key)
		if err != nil {
			return params.RelationLifeSuspendedStatusWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}
		changesParams[i] = *change
	}
	return params.RelationLifeSuspendedStatusWatchResult{
		Changes: changesParams,
	}, nil
}

// srvOfferStatusWatcher defines the API wrapping a
// crossmodelrelations.OfferStatusWatcher.
type srvOfferStatusWatcher struct {
	watcherCommon
	st      *state.State
	watcher crossmodelrelations.OfferWatcher
}

func newOfferStatusWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(crossmodelrelations.OfferWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvOfferStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvOfferStatusWatcher.
func (w *srvOfferStatusWatcher) Next(ctx context.Context) (params.OfferStatusWatchResult, error) {
	_, err := internal.FirstResult[struct{}](ctx, w.watcher)
	if err != nil {
		return params.OfferStatusWatchResult{}, errors.Trace(err)
	}
	change, err := crossmodel.GetOfferStatusChange(
		crossmodel.GetBackend(w.st),
		w.watcher.OfferUUID(), w.watcher.OfferName())
	if err != nil {
		// For the specific case where we are informed that a migration is
		// in progress, we want to return an error that causes the client
		// to stop watching, rather than in the payload.
		if errors.Is(err, migration.ErrMigrating) {
			return params.OfferStatusWatchResult{}, err
		}

		return params.OfferStatusWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.OfferStatusWatchResult{
		Changes: []params.OfferStatusChange{*change},
	}, nil
}

// srvMachineStorageIdsWatcher defines the API wrapping a StringsWatcher
// watching machine/storage attachments. This watcher notifies about storage
// entities (volumes/filesystems) being attached to and detached from machines.
//
// TODO(axw) state needs a new watcher, this is a bt of a hack. State watchers
// could do with some deduplication of logic, and I don't want to add to that
// spaghetti right now.
type srvMachineStorageIdsWatcher struct {
	watcherCommon
	watcher corewatcher.StringsWatcher
	parser  func([]string) ([]params.MachineStorageId, error)
}

func newVolumeAttachmentsWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	return newMachineStorageIdsWatcher(
		context,
		storagecommon.ParseVolumeAttachmentIds,
	)
}

func newVolumeAttachmentPlansWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	return newMachineStorageIdsWatcher(
		context,
		storagecommon.ParseVolumeAttachmentIds,
	)
}

func newFilesystemAttachmentsWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	return newMachineStorageIdsWatcher(
		context,
		storagecommon.ParseFilesystemAttachmentIds,
	)
}

func newMachineStorageIdsWatcher(
	context facade.Context,
	parser func([]string) ([]params.MachineStorageId, error),
) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvMachineStorageIdsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
		parser:        parser,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvMachineStorageIdsWatcher.
func (w *srvMachineStorageIdsWatcher) Next(ctx context.Context) (params.MachineStorageIdsWatchResult, error) {
	stringChanges, err := internal.FirstResult[[]string](ctx, w.watcher)
	if err != nil {
		return params.MachineStorageIdsWatchResult{}, errors.Trace(err)
	}
	changes, err := w.parser(stringChanges)
	if err != nil {
		return params.MachineStorageIdsWatchResult{}, err
	}
	return params.MachineStorageIdsWatchResult{
		Changes: changes,
	}, nil
}

// EntitiesWatcher defines an interface based on the StringsWatcher
// but also providing a method for the mapping of the received
// strings to the tags of the according entities.
type EntitiesWatcher interface {
	corewatcher.StringsWatcher

	// MapChanges maps the received strings to their according tag strings.
	// The EntityFinder interface representing state or a mock has to be
	// upcasted into the needed sub-interface of state for the real mapping.
	MapChanges(in []string) ([]string, error)
}

// srvEntitiesWatcher defines the API for methods on a StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvEntitiesWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings, which could be transformed
// from state entity ids to their corresponding entity tags.
type srvEntitiesWatcher struct {
	watcherCommon
	watcher EntitiesWatcher
}

func newEntitiesWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(EntitiesWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvEntitiesWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvEntitiesWatcher.
func (w *srvEntitiesWatcher) Next(ctx context.Context) (params.EntitiesWatchResult, error) {
	changes, err := internal.FirstResult[[]string](ctx, w.watcher)
	if err != nil {
		return params.EntitiesWatchResult{}, errors.Trace(err)
	}
	mapped, err := w.watcher.MapChanges(changes)
	if err != nil {
		return params.EntitiesWatchResult{}, errors.Annotate(err, "cannot map changes")
	}
	return params.EntitiesWatchResult{
		Changes: mapped,
	}, nil
}

var getMigrationBackend = func(st *state.State) migrationBackend {
	return st
}

var getControllerBackend = func(pool *state.StatePool) (controllerBackend, error) {
	return pool.SystemState()
}

// migrationBackend defines model State functionality required by the
// migration watchers.
type migrationBackend interface {
	LatestMigration() (state.ModelMigration, error)
}

// migrationBackend defines controller State functionality required by the
// migration watchers.
type controllerBackend interface {
	APIHostPortsForClients(controller.Config) ([]network.SpaceHostPorts, error)
}

func newMigrationStatusWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.NotifyWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	var (
		st   = context.State()
		pool = context.StatePool()
	)
	controllerBackend, err := getControllerBackend(pool)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &srvMigrationStatusWatcher{
		watcherCommon:           newWatcherCommon(context),
		watcher:                 watcher,
		st:                      getMigrationBackend(st),
		ctrlSt:                  controllerBackend,
		controllerConfigService: context.ServiceFactory().ControllerConfig(),
	}, nil
}

type srvMigrationStatusWatcher struct {
	watcherCommon
	watcher                 corewatcher.NotifyWatcher
	st                      migrationBackend
	ctrlSt                  controllerBackend
	controllerConfigService ControllerConfigService
}

// Next returns when the status for a model migration for the
// associated model changes. The current details for the active
// migration are returned.
func (w *srvMigrationStatusWatcher) Next(ctx context.Context) (params.MigrationStatus, error) {
	_, err := internal.FirstResult[struct{}](ctx, w.watcher)
	if err != nil {
		return params.MigrationStatus{}, errors.Trace(err)
	}

	mig, err := w.st.LatestMigration()
	if errors.Is(err, errors.NotFound) {
		return params.MigrationStatus{
			Phase: migration.NONE.String(),
		}, nil
	} else if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "migration lookup")
	}

	phase, err := mig.Phase()
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "retrieving migration phase")
	}

	cfg, err := w.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "retrieving controller config")
	}
	sourceAddrs, err := w.getLocalHostPorts(cfg)
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "retrieving source addresses")
	}

	sourceCACert, err := getControllerCACert(cfg)
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "retrieving source CA cert")
	}

	target, err := mig.TargetInfo()
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "retrieving target info")
	}

	return params.MigrationStatus{
		MigrationId:    mig.Id(),
		Attempt:        mig.Attempt(),
		Phase:          phase.String(),
		SourceAPIAddrs: sourceAddrs,
		SourceCACert:   sourceCACert,
		TargetAPIAddrs: target.Addrs,
		TargetCACert:   target.CACert,
	}, nil
}

func (w *srvMigrationStatusWatcher) getLocalHostPorts(cfg controller.Config) ([]string, error) {
	hostports, err := w.ctrlSt.APIHostPortsForClients(cfg)
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

// This is a shim to avoid the need to use a working State into the
// unit tests. It is tested as part of the client side API tests.
var getControllerCACert = func(controllerConfig controller.Config) (string, error) {
	cacert, ok := controllerConfig.CACert()
	if !ok {
		return "", errors.New("missing CA cert for controller model")
	}
	return cacert, nil
}

// newModelSummaryWatcher exists solely to be registered with regRaw.
// Standard registration doesn't handle watcher types (it checks for
// and empty ID in the context).
func newModelSummaryWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	return NewModelSummaryWatcher(context)
}

// NewModelSummaryWatcher returns a new API server endpoint for interacting with
// a watcher created by the WatchModelSummaries and WatchAllModelSummaries API
// calls.
func NewModelSummaryWatcher(context facade.Context) (*SrvModelSummaryWatcher, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		resources       = context.Resources()
		watcherRegistry = context.WatcherRegistry()
	)
	if !auth.AuthClient() {
		// Note that we don't need to check specific permissions
		// here, as the AllWatcher can only do anything if the
		// watcher resource has already been created, so we can
		// rely on the permission check there to ensure that
		// this facade can't do anything it shouldn't be allowed
		// to.
		//
		// This is useful because the AllWatcher is reused for
		// both the WatchAll (requires model access rights) and
		// the WatchAllModels (requring controller superuser
		// rights) API calls.
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(watcherRegistry, resources, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.ModelSummaryWatcher)
	if !ok {
		return nil, errors.Annotatef(apiservererrors.ErrUnknownWatcher, "watcher id: %s", id)
	}
	return &SrvModelSummaryWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// SrvModelSummaryWatcher defines the API methods on a ModelSummaryWatcher.
type SrvModelSummaryWatcher struct {
	watcherCommon
	watcher corewatcher.ModelSummaryWatcher
}

// Next will return the current state of everything on the first call
// and subsequent calls will return just those model summaries that have
// changed.
func (w *SrvModelSummaryWatcher) Next(ctx context.Context) (params.SummaryWatcherNextResults, error) {
	changes, err := internal.FirstResult[[]corewatcher.ModelSummary](ctx, w.watcher)
	if err != nil {
		return params.SummaryWatcherNextResults{}, errors.Trace(err)
	}

	return params.SummaryWatcherNextResults{
		Models: w.translate(changes),
	}, nil
}

func (w *SrvModelSummaryWatcher) translate(summaries []corewatcher.ModelSummary) []params.ModelAbstract {
	response := make([]params.ModelAbstract, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Removed {
			response = append(response, params.ModelAbstract{
				UUID:    summary.UUID,
				Removed: true,
			})
			continue
		}

		result := params.ModelAbstract{
			UUID:       summary.UUID,
			Controller: summary.Controller,
			Name:       summary.Name,
			Admins:     summary.Admins,
			Cloud:      summary.Cloud,
			Region:     summary.Region,
			Credential: summary.Credential,
			Size: params.ModelSummarySize{
				Machines:     summary.MachineCount,
				Containers:   summary.ContainerCount,
				Applications: summary.ApplicationCount,
				Units:        summary.UnitCount,
				Relations:    summary.RelationCount,
			},
			Status:      summary.Status,
			Messages:    w.translateMessages(summary.Messages),
			Annotations: summary.Annotations,
		}
		response = append(response, result)
	}
	return response
}

func (w *SrvModelSummaryWatcher) translateMessages(messages []corewatcher.ModelSummaryMessage) []params.ModelSummaryMessage {
	if messages == nil {
		return nil
	}
	result := make([]params.ModelSummaryMessage, len(messages))
	for i, m := range messages {
		result[i] = params.ModelSummaryMessage{
			Agent:   m.Agent,
			Message: m.Message,
		}
	}
	return result
}

// srvSecretTriggerWatcher defines the API wrapping a SecretTriggerWatcher.
type srvSecretTriggerWatcher struct {
	watcherCommon
	st      *state.State
	watcher corewatcher.SecretTriggerWatcher
}

func newSecretsTriggerWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.SecretTriggerWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvSecretTriggerWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvSecretRotationWatcher.
func (w *srvSecretTriggerWatcher) Next(ctx context.Context) (params.SecretTriggerWatchResult, error) {
	changes, err := internal.FirstResult[[]corewatcher.SecretTriggerChange](ctx, w.watcher)
	if err != nil {
		return params.SecretTriggerWatchResult{}, errors.Trace(err)
	}
	return params.SecretTriggerWatchResult{
		Changes: w.translateChanges(changes),
	}, nil
}

func (w *srvSecretTriggerWatcher) translateChanges(changes []corewatcher.SecretTriggerChange) []params.SecretTriggerChange {
	if changes == nil {
		return nil
	}
	result := make([]params.SecretTriggerChange, len(changes))
	for i, c := range changes {
		result[i] = params.SecretTriggerChange{
			URI:             c.URI.String(),
			Revision:        c.Revision,
			NextTriggerTime: c.NextTriggerTime,
		}
	}
	return result
}

// srvSecretBackendsRotateWatcher defines the API wrapping a SecretBackendsRotateWatcher.
type srvSecretBackendsRotateWatcher struct {
	watcherCommon
	watcher corewatcher.SecretBackendRotateWatcher
}

func newSecretBackendsRotateWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.SecretBackendRotateWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvSecretBackendsRotateWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvSecretRotationWatcher.
func (w *srvSecretBackendsRotateWatcher) Next(ctx context.Context) (params.SecretBackendRotateWatchResult, error) {
	changes, err := internal.FirstResult[[]corewatcher.SecretBackendRotateChange](ctx, w.watcher)
	if err != nil {
		return params.SecretBackendRotateWatchResult{}, errors.Trace(err)
	}
	return params.SecretBackendRotateWatchResult{
		Changes: w.translateChanges(changes),
	}, nil
}

func (w *srvSecretBackendsRotateWatcher) translateChanges(changes []corewatcher.SecretBackendRotateChange) []params.SecretBackendRotateChange {
	if changes == nil {
		return nil
	}
	result := make([]params.SecretBackendRotateChange, len(changes))
	for i, c := range changes {
		result[i] = params.SecretBackendRotateChange{
			ID:              c.ID,
			Name:            c.Name,
			NextTriggerTime: c.NextTriggerTime,
		}
	}
	return result
}

// srvSecretsRevisionWatcher defines the API wrapping a SecretsRevisionWatcher.
type srvSecretsRevisionWatcher struct {
	watcherCommon
	st      *state.State
	watcher corewatcher.StringsWatcher
}

func newSecretsRevisionWatcher(_ context.Context, context facade.Context) (facade.Facade, error) {
	auth := context.Auth()

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := GetWatcherByID(context.WatcherRegistry(), context.Resources(), context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}

	return &srvSecretsRevisionWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvSecretRotationWatcher.
func (w *srvSecretsRevisionWatcher) Next(ctx context.Context) (params.SecretRevisionWatchResult, error) {
	changes, err := internal.FirstResult[[]string](ctx, w.watcher)
	if err != nil {
		return params.SecretRevisionWatchResult{}, errors.Trace(err)
	}
	ch, err := w.translateChanges(changes)
	if err != nil {
		return params.SecretRevisionWatchResult{}, errors.Trace(err)
	}
	return params.SecretRevisionWatchResult{
		Changes: ch,
	}, nil
}

func (w *srvSecretsRevisionWatcher) translateChanges(changes []string) ([]params.SecretRevisionChange, error) {
	if changes == nil {
		return nil, nil
	}
	secrets := state.NewSecrets(w.st)
	result := make([]params.SecretRevisionChange, len(changes))
	for i, uriStr := range changes {
		uri, err := coresecrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		md, err := secrets.GetSecret(uri)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[i] = params.SecretRevisionChange{
			URI:      uri.String(),
			Revision: md.LatestRevision,
		}
	}
	return result, nil
}
