// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.remoterelations")

func init() {
	common.RegisterStandardFacadeForFeature("RemoteRelations", 1, NewStateRemoteRelationsAPI, feature.CrossModelRelations)
}

// RemoteRelationsAPI provides access to the Provisioner API facade.
type RemoteRelationsAPI struct {
	st         RemoteRelationsState
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewRemoteRelationsAPI creates a new server-side RemoteRelationsAPI facade
// backed by global state.
func NewStateRemoteRelationsAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*RemoteRelationsAPI, error) {
	return NewRemoteRelationsAPI(stateShim{st}, resources, authorizer)
}

// NewRemoteRelationsAPI returns a new server-side RemoteRelationsAPI facade.
func NewRemoteRelationsAPI(
	st RemoteRelationsState,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*RemoteRelationsAPI, error) {
	if !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	return &RemoteRelationsAPI{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// ExportEntities allocates unique, remote entity IDs for the given entities in the local model.
func (api *RemoteRelationsAPI) ExportEntities(entities params.Entities) (params.RemoteEntityIdResults, error) {
	results := params.RemoteEntityIdResults{
		Results: make([]params.RemoteEntityIdResult, len(entities.Entities)),
	}
	for i, entity := range entities.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		token, err := api.st.ExportLocalEntity(tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			if !errors.IsAlreadyExists(err) {
				continue
			}
		}
		results.Results[i].Result = &params.RemoteEntityId{
			ModelUUID: api.st.ModelUUID(),
			Token:     token,
		}
	}
	return results, nil
}

// RelationUnitSettings returns the relation unit settings for the given relation units in the local model.
func (api *RemoteRelationsAPI) RelationUnitSettings(relationUnits params.RelationUnits) (params.SettingsResults, error) {
	results := params.SettingsResults{
		Results: make([]params.SettingsResult, len(relationUnits.RelationUnits)),
	}
	one := func(ru params.RelationUnit) (params.Settings, error) {
		relationTag, err := names.ParseRelationTag(ru.Relation)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rel, err := api.st.KeyRelation(relationTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		unitTag, err := names.ParseUnitTag(ru.Unit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		unit, err := rel.Unit(unitTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		settings, err := unit.Settings()
		if err != nil {
			return nil, errors.Trace(err)
		}
		paramsSettings := make(params.Settings)
		for k, v := range settings {
			vString, ok := v.(string)
			if !ok {
				return nil, errors.Errorf(
					"invalid relation setting %q: expected string, got %T", k, v,
				)
			}
			paramsSettings[k] = vString
		}
		return paramsSettings, nil
	}
	for i, ru := range relationUnits.RelationUnits {
		settings, err := one(ru)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Settings = settings
	}
	return results, nil
}

// Relations returns information about the cross-model relations with the specified keys
// in the local model.
func (api *RemoteRelationsAPI) Relations(entities params.Entities) (params.RelationResults, error) {
	results := params.RelationResults{
		Results: make([]params.RelationResult, len(entities.Entities)),
	}
	one := func(entity params.Entity) (params.RelationResult, error) {
		tag, err := names.ParseRelationTag(entity.Tag)
		if err != nil {
			return params.RelationResult{}, errors.Trace(err)
		}
		rel, err := api.st.KeyRelation(tag.Id())
		if err != nil {
			return params.RelationResult{}, errors.Trace(err)
		}
		return params.RelationResult{
			Id:   rel.Id(),
			Life: params.Life(rel.Life().String()),
			Key:  tag.Id(),
		}, nil
	}
	for i, entity := range entities.Entities {
		remoteRelation, err := one(entity)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i] = remoteRelation
	}
	return results, nil
}

// RemoteApplications returns the current state of the remote applications with
// the specified names in the local model.
func (api *RemoteRelationsAPI) RemoteApplications(entities params.Entities) (params.RemoteApplicationResults, error) {
	results := params.RemoteApplicationResults{
		Results: make([]params.RemoteApplicationResult, len(entities.Entities)),
	}
	one := func(entity params.Entity) (*params.RemoteApplication, error) {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		remoteApp, err := api.st.RemoteApplication(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		status, err := remoteApp.Status()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &params.RemoteApplication{
			Name:      remoteApp.Name(),
			Life:      params.Life(remoteApp.Life().String()),
			Status:    status.Status.String(),
			ModelUUID: remoteApp.SourceModel().Id(),
		}, nil
	}
	for i, entity := range entities.Entities {
		remoteApplication, err := one(entity)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = remoteApplication
	}
	return results, nil
}

// ConsumeRemoteApplicationChange consumes remote changes to applications into the local model.
func (api *RemoteRelationsAPI) ConsumeRemoteApplicationChange(
	changes params.RemoteApplicationChanges,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}
	handleRemoteRelationsChange := func(change params.RemoteRelationsChange) error {
		// For any relations that have been removed on the offering
		// side, destroy them on the consuming side.
		for _, relId := range change.RemovedRelations {
			rel, err := api.st.Relation(relId)
			if errors.IsNotFound(err) {
				continue
			} else if err != nil {
				return errors.Trace(err)
			}
			if err := rel.Destroy(); err != nil {
				return errors.Trace(err)
			}
			// TODO(axw) remove remote relation units.
		}
		for _, change := range change.ChangedRelations {
			rel, err := api.st.Relation(change.RelationId)
			if err != nil {
				return errors.Trace(err)
			}
			if change.Life != params.Alive {
				if err := rel.Destroy(); err != nil {
					return errors.Trace(err)
				}
			}
			for _, unitId := range change.DepartedUnits {
				ru, err := rel.RemoteUnit(unitId)
				if err != nil {
					return errors.Trace(err)
				}
				if err := ru.LeaveScope(); err != nil {
					return errors.Trace(err)
				}
			}
			for unitId, change := range change.ChangedUnits {
				ru, err := rel.RemoteUnit(unitId)
				if err != nil {
					return errors.Trace(err)
				}
				inScope, err := ru.InScope()
				if err != nil {
					return errors.Trace(err)
				}
				if !inScope {
					err = ru.EnterScope(change.Settings)
				} else {
					err = ru.ReplaceSettings(change.Settings)
				}
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
		return nil
	}
	handleApplicationChange := func(change params.RemoteApplicationChange) error {
		applicationTag, err := names.ParseApplicationTag(change.ApplicationTag)
		if err != nil {
			return errors.Trace(err)
		}
		application, err := api.st.RemoteApplication(applicationTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		// TODO(axw) update application status, lifecycle state.
		_ = application
		return handleRemoteRelationsChange(change.Relations)
	}
	for i, change := range changes.Changes {
		if err := handleApplicationChange(change); err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

// PublishLocalRelationChange publishes local relations changes to the
// remote side offering those relations.
func (api *RemoteRelationsAPI) PublishLocalRelationChange(
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	return params.ErrorResults{}, errors.NotImplementedf("PublishLocalRelationChange")
}

// RegisterRemoteRelations sets up the local model to participate
// in the specified relations. This operation is idempotent.
func (api *RemoteRelationsAPI) RegisterRemoteRelations(
	relations params.RegisterRemoteRelations,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(relations.Relations)),
	}
	for i, relation := range relations.Relations {
		if err := api.registerRemoteRelation(relation); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}

func (api *RemoteRelationsAPI) registerRemoteRelation(relation params.RegisterRemoteRelation) error {
	// TODO(wallyworld) - do this as a transaction so the result is atomic
	// Perform some initial validation - is the local application alive?

	// TODO(wallyworld) - look up local name from offered name
	localApplicationName := relation.OfferedApplicationName

	localApp, err := api.st.Application(localApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	if localApp.Life() != state.Alive {
		return errors.NotFoundf("application %v", localApplicationName)
	}
	eps, err := localApp.Endpoints()
	if err != nil {
		return errors.Trace(err)
	}

	// Does the requested local endpoint exist?
	var localEndpoint *state.Endpoint
	for _, ep := range eps {
		if ep.Name == relation.LocalEndpointName {
			localEndpoint = &ep
			break
		}
	}
	if localEndpoint == nil {
		return errors.NotFoundf("relation endpoint %v", relation.LocalEndpointName)
	}

	// Add the remote application reference. We construct a unique, opaque application name based on the
	// token passed in from the consuming model. This model, which is offering the application being
	// related to, does not need to know the name of the consuming application.
	uniqueRemoteApplicationName := "remote-" + strings.Replace(relation.ApplicationId.Token, "-", "", -1)
	remoteEndpoint := state.Endpoint{
		ApplicationName: uniqueRemoteApplicationName,
		Relation: charm.Relation{
			Name:      relation.RemoteEndpoint.Name,
			Scope:     relation.RemoteEndpoint.Scope,
			Interface: relation.RemoteEndpoint.Interface,
			Role:      relation.RemoteEndpoint.Role,
			Limit:     relation.RemoteEndpoint.Limit,
		},
	}

	remoteModelTag := names.NewModelTag(relation.ApplicationId.ModelUUID)
	_, err = api.st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        uniqueRemoteApplicationName,
		SourceModel: names.NewModelTag(relation.ApplicationId.ModelUUID),
		Token:       relation.ApplicationId.Token,
		Endpoints:   []charm.Relation{remoteEndpoint.Relation},
	})
	// If it already exists, that's fine.
	if err != nil && !errors.IsAlreadyExists(err) {
		return errors.Annotatef(err, "adding remote application %v", uniqueRemoteApplicationName)
	}
	logger.Debugf("added remote application %v to local model", uniqueRemoteApplicationName)

	// Now add the relation.
	_, err = api.st.AddRelation(*localEndpoint, remoteEndpoint)
	// Again, if it already exists, that's fine.
	if err != nil && !errors.IsAlreadyExists(err) {
		return errors.Annotate(err, "adding remote relation")
	}
	localRel, err := api.st.EndpointsRelation(*localEndpoint, remoteEndpoint)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("added remote relation %v to local environment", localRel.Id())

	// Ensure we have references recorded.
	err = api.st.ImportRemoteEntity(remoteModelTag, names.NewApplicationTag(uniqueRemoteApplicationName), relation.ApplicationId.Token)
	if err != nil && !errors.IsAlreadyExists(err) {
		return errors.Annotatef(err, "importing remote application %v to local model", uniqueRemoteApplicationName)
	}
	err = api.st.ImportRemoteEntity(remoteModelTag, localRel.Tag(), relation.RelationId.Token)
	if err != nil && !errors.IsAlreadyExists(err) {
		return errors.Annotatef(err, "importing remote relation %v to local model", localRel.Tag().Id())
	}
	return nil
}

// WatchRemoteApplications starts a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote applications in the model; and
// returns the watcher ID and initial IDs of remote applications, or an error if
// watching failed.
func (api *RemoteRelationsAPI) WatchRemoteApplications() (params.StringsWatchResult, error) {
	w := api.st.WatchRemoteApplications()
	if changes, ok := <-w.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: api.resources.Register(w),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(w)
}

// WatchLocalRelationUnits starts a RelationUnitsWatcher for watching the local
// relation units involved in each specified relation in the local model,
// and returns the watcher IDs and initial values, or an error if the relation
// units could not be watched.
func (api *RemoteRelationsAPI) WatchLocalRelationUnits(args params.Entities) (params.RelationUnitsWatchResults, error) {
	results := params.RelationUnitsWatchResults{
		make([]params.RelationUnitsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		relationTag, err := names.ParseRelationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		w, err := api.watchLocalRelationUnits(relationTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}
		results.Results[i].RelationUnitsWatcherId = api.resources.Register(w)
		results.Results[i].Changes = changes
	}
	return results, nil
}

func (api *RemoteRelationsAPI) watchLocalRelationUnits(tag names.RelationTag) (state.RelationUnitsWatcher, error) {
	relation, err := api.st.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, ep := range relation.Endpoints() {
		_, err := api.st.Application(ep.ApplicationName)
		if errors.IsNotFound(err) {
			// Not found, so it's the remote application. Try the next endpoint.
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		w, err := relation.WatchUnits(ep.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return w, nil
	}
	return nil, errors.NotFoundf("local application for %s", names.ReadableString(tag))
}

// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
// each specified application in the local model, and returns the watcher IDs
// and initial values, or an error if the services' relations could not be
// watched.
func (api *RemoteRelationsAPI) WatchRemoteApplicationRelations(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		applicationTag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		appName := applicationTag.Id()
		w, err := api.st.WatchRemoteApplicationRelations(appName)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}
		results.Results[i].StringsWatcherId = api.resources.Register(w)
		results.Results[i].Changes = changes
	}
	return results, nil
}

// TODO(wallyworld) - the the stuff below is currently used.

func (api *RemoteRelationsAPI) watchApplication(applicationTag names.ApplicationTag) (*applicationRelationsWatcher, error) {
	// TODO(axw) subscribe to changes sent by the offering side.
	applicationName := applicationTag.Id()
	relationsWatcher, err := api.st.WatchRemoteApplicationRelations(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newRemoteRelationsWatcher(api.st, applicationName, relationsWatcher), nil
}

// applicationRelationsWatcher watches the relations of a application, and the
// *counterpart* endpoint units for each of those relations.
type applicationRelationsWatcher struct {
	tomb                  tomb.Tomb
	st                    RemoteRelationsState
	applicationName       string
	relationsWatcher      state.StringsWatcher
	relationUnitsChanges  chan relationUnitsChange
	relationUnitsWatchers map[string]*relationWatcher
	relations             map[string]relationInfo
	out                   chan params.RemoteRelationsChange
}

func newRemoteRelationsWatcher(
	st RemoteRelationsState,
	applicationName string,
	rw state.StringsWatcher,
) *applicationRelationsWatcher {
	w := &applicationRelationsWatcher{
		st:                    st,
		applicationName:       applicationName,
		relationsWatcher:      rw,
		relationUnitsChanges:  make(chan relationUnitsChange),
		relationUnitsWatchers: make(map[string]*relationWatcher),
		relations:             make(map[string]relationInfo),
		out:                   make(chan params.RemoteRelationsChange),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		defer close(w.relationUnitsChanges)
		defer watcher.Stop(rw, &w.tomb)
		defer func() {
			for _, ruw := range w.relationUnitsWatchers {
				watcher.Stop(ruw, &w.tomb)
			}
		}()
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *applicationRelationsWatcher) loop() error {
	var out chan<- params.RemoteRelationsChange
	var value params.RemoteRelationsChange
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case change, ok := <-w.relationsWatcher.Changes():
			if !ok {
				return watcher.EnsureErr(w.relationsWatcher)
			}
			for _, relationKey := range change {
				relation, err := w.st.KeyRelation(relationKey)
				if errors.IsNotFound(err) {
					r, ok := w.relations[relationKey]
					if !ok {
						// Relation was not previously known, so
						// don't report it as removed.
						continue
					}
					delete(w.relations, relationKey)
					relationId := r.relationId

					// Relation has been removed, so stop and remove its
					// relation units watcher, and then add the relation
					// ID to the removed relations list.
					watcher, ok := w.relationUnitsWatchers[relationKey]
					if ok {
						if err := watcher.Stop(); err != nil {
							return errors.Trace(err)
						}
						delete(w.relationUnitsWatchers, relationKey)
					}
					value.RemovedRelations = append(
						value.RemovedRelations, relationId,
					)
					continue
				} else if err != nil {
					return errors.Trace(err)
				}

				relationId := relation.Id()
				relationChange, _ := getRelationChange(&value, relationId)
				relationChange.Life = params.Life(relation.Life().String())
				w.relations[relationKey] = relationInfo{relationId, relationChange.Life}
				if _, ok := w.relationUnitsWatchers[relationKey]; !ok {
					// Start a relation units watcher, wait for the initial
					// value before informing the client of the relation.
					ruw, err := relation.WatchUnits(w.applicationName)
					if err != nil {
						return errors.Trace(err)
					}
					var knownUnits set.Strings
					select {
					case <-w.tomb.Dying():
						return tomb.ErrDying
					case change, ok := <-ruw.Changes():
						if !ok {
							return watcher.EnsureErr(ruw)
						}
						ru := relationUnitsChange{
							relationKey: relationKey,
						}
						knownUnits = make(set.Strings)
						if err := updateRelationUnits(
							w.st, relation, knownUnits, change, &ru,
						); err != nil {
							watcher.Stop(ruw, &w.tomb)
							return errors.Trace(err)
						}
						w.updateRelationUnits(ru, &value)
					}
					w.relationUnitsWatchers[relationKey] = newRelationWatcher(
						w.st, relation, relationKey, knownUnits,
						ruw, w.relationUnitsChanges,
					)
				}
			}
			out = w.out

		case change := <-w.relationUnitsChanges:
			w.updateRelationUnits(change, &value)
			out = w.out

		case out <- value:
			out = nil
			value = params.RemoteRelationsChange{}
		}
	}
}

func (w *applicationRelationsWatcher) updateRelationUnits(change relationUnitsChange, value *params.RemoteRelationsChange) {
	relationInfo, ok := w.relations[change.relationKey]
	r, ok := getRelationChange(value, relationInfo.relationId)
	if !ok {
		r.Life = relationInfo.life
	}
	if r.ChangedUnits == nil && len(change.changedUnits) > 0 {
		r.ChangedUnits = make(map[string]params.RemoteRelationUnitChange)
	}
	for unitId, unitChange := range change.changedUnits {
		r.ChangedUnits[unitId] = unitChange
	}
	if r.ChangedUnits != nil {
		for _, unitId := range change.departedUnits {
			delete(r.ChangedUnits, unitId)
		}
	}
	r.DepartedUnits = append(r.DepartedUnits, change.departedUnits...)
}

func getRelationChange(value *params.RemoteRelationsChange, relationId int) (*params.RemoteRelationChange, bool) {
	for i, r := range value.ChangedRelations {
		if r.RelationId == relationId {
			return &value.ChangedRelations[i], true
		}
	}
	value.ChangedRelations = append(
		value.ChangedRelations, params.RemoteRelationChange{RelationId: relationId},
	)
	return &value.ChangedRelations[len(value.ChangedRelations)-1], false
}

func (w *applicationRelationsWatcher) updateRelation(change params.RemoteRelationChange, value *params.RemoteRelationsChange) {
	for i, r := range value.ChangedRelations {
		if r.RelationId == change.RelationId {
			value.ChangedRelations[i] = change
			return
		}
	}
}

func (w *applicationRelationsWatcher) Changes() <-chan params.RemoteRelationsChange {
	return w.out
}

func (w *applicationRelationsWatcher) Err() error {
	return w.tomb.Err()
}

func (w *applicationRelationsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// relationWatcher watches the counterpart endpoint units for a relation.
type relationWatcher struct {
	tomb        tomb.Tomb
	st          RemoteRelationsState
	relation    Relation
	relationKey string
	knownUnits  set.Strings
	watcher     state.RelationUnitsWatcher
	out         chan<- relationUnitsChange
}

func newRelationWatcher(
	st RemoteRelationsState,
	relation Relation,
	relationKey string,
	knownUnits set.Strings,
	ruw state.RelationUnitsWatcher,
	out chan<- relationUnitsChange,
) *relationWatcher {
	w := &relationWatcher{
		st:          st,
		relation:    relation,
		relationKey: relationKey,
		knownUnits:  knownUnits,
		watcher:     ruw,
		out:         out,
	}
	go func() {
		defer w.tomb.Done()
		defer watcher.Stop(ruw, &w.tomb)
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *relationWatcher) loop() error {
	value := relationUnitsChange{relationKey: w.relationKey}
	var out chan<- relationUnitsChange
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case change, ok := <-w.watcher.Changes():
			if !ok {
				return watcher.EnsureErr(w.watcher)
			}
			if err := w.update(change, &value); err != nil {
				return errors.Trace(err)
			}
			out = w.out

		case out <- value:
			out = nil
			value = relationUnitsChange{relationKey: w.relationKey}
		}
	}
}

func (w *relationWatcher) update(change params.RelationUnitsChange, value *relationUnitsChange) error {
	return updateRelationUnits(w.st, w.relation, w.knownUnits, change, value)
}

// updateRelationUnits updates a relationUnitsChange structure with the
// params.RelationUnitsChange.
func updateRelationUnits(
	st RemoteRelationsState,
	relation Relation,
	knownUnits set.Strings,
	change params.RelationUnitsChange,
	value *relationUnitsChange,
) error {
	if value.changedUnits == nil && len(change.Changed) > 0 {
		value.changedUnits = make(map[string]params.RemoteRelationUnitChange)
	}
	if value.changedUnits != nil {
		for _, unitId := range change.Departed {
			delete(value.changedUnits, unitId)
		}
	}
	for _, unitId := range change.Departed {
		if knownUnits == nil || !knownUnits.Contains(unitId) {
			// Unit hasn't previously been seen. This could happen
			// if the unit is removed between the watcher firing
			// when it was present and reading the unit's settings.
			continue
		}
		knownUnits.Remove(unitId)
		value.departedUnits = append(value.departedUnits, unitId)
	}

	// Fetch settings for each changed relation unit.
	for unitId := range change.Changed {
		ru, err := relation.Unit(unitId)
		if errors.IsNotFound(err) {
			// Relation unit removed between watcher firing and
			// reading the unit's settings.
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
		settings, err := ru.Settings()
		if err != nil {
			return errors.Trace(err)
		}
		value.changedUnits[unitId] = params.RemoteRelationUnitChange{Settings: settings}
		if knownUnits != nil {
			knownUnits.Add(unitId)
		}
	}
	return nil
}

func (w *relationWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *relationWatcher) Err() error {
	return w.tomb.Err()
}

type relationInfo struct {
	relationId int
	life       params.Life
}

type relationUnitsChange struct {
	relationKey   string
	changedUnits  map[string]params.RemoteRelationUnitChange
	departedUnits []string
}
