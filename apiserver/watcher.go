// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/internal"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/watcherregistry"
	"github.com/juju/juju/rpc/params"
)

type watcherCommon struct {
	id              string
	watcherRegistry watcherregistry.WatcherRegistry
	dispose         func()
}

func newWatcherCommon(context facade.ModelContext) watcherCommon {
	return watcherCommon{
		id:              context.ID(),
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
	return nil
}

func isAgent(auth facade.Authorizer) bool {
	return auth.AuthMachineAgent() || auth.AuthUnitAgent() || auth.AuthApplicationAgent() || auth.AuthModelAgent()
}

func isAgentOrUser(auth facade.Authorizer) bool {
	return isAgent(auth) || auth.AuthClient()
}

func newNotifyWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support anonymous api calls
	// with macaroons.
	if auth.GetAuthTag() != nil && !isAgentOrUser(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := context.WatcherRegistry().Get(context.ID())
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

func newStringsWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support anonymous api calls
	// with macaroons.
	if auth.GetAuthTag() != nil && !isAgentOrUser(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := context.WatcherRegistry().Get(context.ID())
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

// Next returns when a change has occurred to an entity of the collection being
// watched since the most recent call to Next or the Watch call that created the
// srvStringsWatcher.
func (w *srvStringsWatcher) Next(ctx context.Context) (params.StringsWatchResult, error) {
	changes, err := internal.FirstResult[[]string](ctx, w.watcher)
	if err != nil {
		return params.StringsWatchResult{}, errors.Trace(err)
	}
	return params.StringsWatchResult{
		Changes: changes,
	}, nil
}

// srvRelationUnitsWatcher defines the API wrapping a RelationUnitsWatcher. It
// notifies about units entering and leaving the scope of a RelationUnit, and
// changes to the settings of those units known to have entered.
type srvRelationUnitsWatcher struct {
	watcherCommon
	watcher common.RelationUnitsWatcher
}

func newRelationUnitsWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	auth := context.Auth()
	// TODO(wallyworld) - enhance this watcher to support anonymous api calls
	// with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := context.WatcherRegistry().Get(context.ID())
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

// Next returns when a change has occurred to an entity of the collection being
// watched since the most recent call to Next or the Watch call that created the
// srvRelationUnitsWatcher.
func (w *srvRelationUnitsWatcher) Next(ctx context.Context) (params.RelationUnitsWatchResult, error) {
	changes, err := internal.FirstResult(ctx, w.watcher)
	if err != nil {
		return params.RelationUnitsWatchResult{}, errors.Trace(err)
	}
	return params.RelationUnitsWatchResult{
		Changes: changes,
	}, nil
}

// srvRemoteRelationWatcher defines the API wrapping a RelationUnitsWatcher but
// serving the events it emits as fully-expanded
// params.RemoteRelationChangeEvents so they can be used across model/controller
// boundaries.
type srvRemoteRelationWatcher struct {
	watcherCommon
	watcher         crossmodelrelations.RelationChangesWatcher
	relationService RelationService
}

func newRemoteRelationWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	// TODO(wallyworld) - enhance this watcher to support anonymous api calls
	// with macaroons.
	auth := context.Auth()
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}

	id := context.ID()
	watcherRegistry := context.WatcherRegistry()

	w, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(crossmodelrelations.RelationChangesWatcher)
	if !ok {
		return nil, errors.Errorf("watcher id: %s is not a crossmodelrelations.RelationChangesWatcher", id)
	}

	domainServices := context.DomainServices()

	return &srvRemoteRelationWatcher{
		watcherCommon:   newWatcherCommon(context),
		watcher:         watcher,
		relationService: domainServices.Relation(),
	}, nil
}

func (w *srvRemoteRelationWatcher) Next(ctx context.Context) (params.RemoteRelationWatchResult, error) {
	select {
	case <-ctx.Done():
		return params.RemoteRelationWatchResult{}, ctx.Err()
	case change, ok := <-w.watcher.Changes():
		if !ok {
			return params.RemoteRelationWatchResult{}, apiservererrors.ErrStoppedWatcher
		}

		var departed []int
		for _, unitName := range change.Departed {
			num := unit.Name(unitName).Number()
			departed = append(departed, num)
		}

		relationUUID := w.watcher.RelationToken()
		applicationUUID := w.watcher.ApplicationToken()

		inScopeUnitNames, err := w.relationService.GetInScopeUnits(ctx, applicationUUID, relationUUID)
		if err != nil {
			return params.RemoteRelationWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		changedUnitNames := transform.MapToSlice(change.Changed,
			func(k string, _ params.UnitSettings) []unit.Name { return []unit.Name{unit.Name(k)} })

		changedUnitSettings, err := w.relationService.GetUnitSettingsForUnits(ctx, relationUUID, changedUnitNames)
		if err != nil {
			return params.RemoteRelationWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}
		changedUnitSettingsParams := transform.Slice(changedUnitSettings,
			func(in relation.UnitSettings) params.RemoteRelationUnitChange {
				return params.RemoteRelationUnitChange{
					UnitId:   in.UnitID,
					Settings: transform.Map(in.Settings, func(k string, v string) (string, interface{}) { return k, v }),
				}
			})

		var appSettings map[string]string
		if len(change.AppChanged) > 0 {
			var err error
			appSettings, err = w.relationService.GetRelationApplicationSettings(ctx, relationUUID, applicationUUID)
			if err != nil {
				return params.RemoteRelationWatchResult{
					Error: apiservererrors.ServerError(err),
				}, nil
			}
		}

		return params.RemoteRelationWatchResult{
			Changes: params.RemoteRelationChangeEvent{
				RelationToken:           relationUUID.String(),
				ApplicationOrOfferToken: applicationUUID.String(),
				DepartedUnits:           departed,
				InScopeUnits:            transform.Slice(inScopeUnitNames, func(n unit.Name) int { return n.Number() }),
				UnitCount:               len(inScopeUnitNames),
				ApplicationSettings:     transform.Map(appSettings, func(k string, v string) (string, interface{}) { return k, v }),
				ChangedUnits:            changedUnitSettingsParams,
			},
		}, nil
	}
}

// srvRelationStatusWatcher defines the API wrapping a RelationStatusWatcher.
type srvRelationStatusWatcher struct {
	watcherCommon
	watcher         crossmodelrelations.RelationStatusWatcher
	relationService RelationService
}

func newRelationStatusWatcher(ctx context.Context, context facade.ModelContext) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()

	// TODO(wallyworld Oct 2017) - enhance this watcher to support
	// anonymous api calls with macaroons. (All watchers in the file, see 3.6)
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}

	watcherRegistry := context.WatcherRegistry()
	w, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	watcher, ok := w.(crossmodelrelations.RelationStatusWatcher)
	if !ok {
		return nil, internalerrors.Errorf("watcher id: %q is not a RelationStatusWatcher", id).Add(apiservererrors.ErrUnknownWatcher)
	}

	return &srvRelationStatusWatcher{
		watcherCommon:   newWatcherCommon(context),
		relationService: context.DomainServices().Relation(),
		watcher:         watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationStatusWatcher.
func (w *srvRelationStatusWatcher) Next(ctx context.Context) (params.RelationLifeSuspendedStatusWatchResult, error) {
	select {
	case <-ctx.Done():
		return params.RelationLifeSuspendedStatusWatchResult{}, ctx.Err()
	case _, ok := <-w.watcher.Changes():
		if !ok {
			return params.RelationLifeSuspendedStatusWatchResult{}, apiservererrors.ErrStoppedWatcher
		}

		// TODO (hml) only send the change if not migrating
		// If we are migrating, we do not want to inform remote watchers that
		// the relation is dead before they have had a chance to be redirected
		// to the new controller. Check other watchers in this file as well.
		relationUUID := w.watcher.RelationUUID()
		change, err := w.relationService.GetRelationLifeSuspendedStatus(ctx, relationUUID)
		if err != nil {
			return params.RelationLifeSuspendedStatusWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		return params.RelationLifeSuspendedStatusWatchResult{
			Changes: []params.RelationLifeSuspendedStatusChange{
				{
					Key:             change.Key,
					Life:            change.Life,
					Suspended:       change.Suspended,
					SuspendedReason: change.SuspendedReason,
				},
			},
		}, nil
	}
}

// srvOfferStatusWatcher defines the API wrapping a
// crossmodelrelations.OfferStatusWatcher.
type srvOfferStatusWatcher struct {
	watcherCommon
	watcher       crossmodelrelations.OfferWatcher
	statusService StatusService
}

func newOfferStatusWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	id := context.ID()
	watcherRegistry := context.WatcherRegistry()

	w, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(crossmodelrelations.OfferWatcher)
	if !ok {
		return nil, errors.Errorf("watcher id: %q is not a OfferWatcher", id)
	}

	domainServices := context.DomainServices()

	return &srvOfferStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
		statusService: domainServices.Status(),
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvOfferStatusWatcher.
func (w *srvOfferStatusWatcher) Next(ctx context.Context) (params.OfferStatusWatchResult, error) {
	select {
	case <-ctx.Done():
		return params.OfferStatusWatchResult{}, ctx.Err()
	case _, ok := <-w.watcher.Changes():
		if !ok {
			return params.OfferStatusWatchResult{}, apiservererrors.ErrStoppedWatcher
		}
		offerUUID := w.watcher.OfferUUID()
		status, err := w.statusService.GetOfferStatus(ctx, offerUUID)
		if err != nil {
			return params.OfferStatusWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		return params.OfferStatusWatchResult{
			Changes: []params.OfferStatusChange{
				{
					OfferUUID: offerUUID.String(),
					Status: params.EntityStatus{
						Status: status.Status,
						Info:   status.Message,
						Data:   status.Data,
						Since:  status.Since,
					},
				},
			},
		}, nil
	}
}

// EntitiesWatcher defines an interface based on the StringsWatcher
// but also providing a method for the mapping of the received
// strings to the tags of the according entities.
type EntitiesWatcher interface {
	corewatcher.StringsWatcher

	// MapChanges maps the received strings to their according tag strings.
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

func newEntitiesWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := context.WatcherRegistry().Get(context.ID())
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

// newModelSummaryWatcher exists solely to be registered with regRaw.
// Standard registration doesn't handle watcher types (it checks for
// and empty ID in the context).
func newModelSummaryWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	return NewModelSummaryWatcher(context)
}

// NewModelSummaryWatcher returns a new API server endpoint for interacting with
// a watcher created by the WatchModelSummaries and WatchAllModelSummaries API
// calls.
func NewModelSummaryWatcher(context facade.ModelContext) (*SrvModelSummaryWatcher, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
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
		// the WatchAllModels (requiring controller superuser
		// rights) API calls.
		return nil, apiservererrors.ErrPerm
	}
	w, err := watcherRegistry.Get(context.ID())
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
	watcher corewatcher.SecretTriggerWatcher
}

func newSecretsTriggerWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := context.WatcherRegistry().Get(context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.SecretTriggerWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvSecretTriggerWatcher{
		watcherCommon: newWatcherCommon(context),
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

func newSecretBackendsRotateWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	auth := context.Auth()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := context.WatcherRegistry().Get(context.ID())
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

type secretService interface {
	GetLatestRevisions(ctx context.Context, uris []*coresecrets.URI) (map[string]int, error)
}

// srvSecretsRevisionWatcher defines the API wrapping a SecretsRevisionWatcher.
type srvSecretsRevisionWatcher struct {
	watcherCommon
	secretService secretService
	watcher       corewatcher.StringsWatcher
}

func newSecretsRevisionWatcher(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
	auth := context.Auth()

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := context.WatcherRegistry().Get(context.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}

	return &srvSecretsRevisionWatcher{
		watcherCommon: newWatcherCommon(context),
		secretService: context.DomainServices().Secret(),
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
	ch, err := w.translateChanges(ctx, changes)
	if err != nil {
		return params.SecretRevisionWatchResult{}, errors.Trace(err)
	}
	return params.SecretRevisionWatchResult{
		Changes: ch,
	}, nil
}

func (w *srvSecretsRevisionWatcher) translateChanges(ctx context.Context, changes []string) ([]params.SecretRevisionChange, error) {
	if changes == nil {
		return nil, nil
	}
	uris := make([]*coresecrets.URI, len(changes))
	for i, s := range changes {
		uri, err := coresecrets.ParseURI(s)
		if err != nil {
			return nil, errors.Trace(err)
		}
		uris[i] = uri
	}
	latest, err := w.secretService.GetLatestRevisions(ctx, uris)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]params.SecretRevisionChange, len(uris))
	for i, uri := range uris {
		result[i] = params.SecretRevisionChange{
			URI:            uri.String(),
			LatestRevision: latest[uri.ID],
		}
	}
	return result, nil
}
