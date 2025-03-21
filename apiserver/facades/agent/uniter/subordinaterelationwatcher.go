// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/worker/v4/catacomb"

	coreapplication "github.com/juju/juju/core/application"
	corelogger "github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	internalrelation "github.com/juju/juju/internal/relation"
	"github.com/juju/juju/state"
)

// LegacyBackend describes state methods still required by the
// subordinateRelationWatcher.
type LegacyBackend interface {
	Application(string) (LegacyApplicationBackend, error)
}

// LegacyApplicationBackend describes state application methods still required
// by the subordinateRelationWatcher.
type LegacyApplicationBackend interface {
	IsPrincipal() bool
}

type legacyBackendShim struct {
	st *state.State
}

func (l *legacyBackendShim) Application(name string) (LegacyApplicationBackend, error) {
	return l.st.Application(name)
}

// SRWApplicationService describes methods required from the application domain
// by the subordinateRelationWatcher.
type SRWApplicationService interface {
	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	//
	// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
	// and [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)
}

// SRWRelationService describes methods required from the relation domain
// by the subordinateRelationWatcher.
type SRWRelationService interface {
	// GetRelatedEndpoints returns the endpoints of the relation with which
	// units of the named application will establish relations.
	GetRelatedEndpoints(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationName string,
	) ([]internalrelation.Endpoint, error)

	// GetRelationEndpoint returns the endpoint for the given application and
	// relation identifier combination.
	GetRelationEndpoint(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID coreapplication.ID,
	) (internalrelation.Endpoint, error)

	// GetRelationUUIDFromKey returns a relation UUID for the given relation
	// Key. The relation key is a ordered space separated string of the
	// endpoint names of a the relation.
	GetRelationUUIDFromKey(ctx context.Context, relationKey corerelation.Key) (corerelation.UUID, error)

	// WatchLifeSuspendedStatus returns a watcher that notifies of changes to the life
	// or suspended status any relation the application is part of.
	WatchLifeSuspendedStatus(ctx context.Context, applicationID coreapplication.ID) (watcher.StringsWatcher, error)
}

type subRelationsWatcher struct {
	catacomb        catacomb.Catacomb
	principalName   string
	subordinateName string

	backend LegacyBackend

	applicationService SRWApplicationService
	relationService    SRWRelationService

	// Maps relation keys to whether that relation should be
	// included. Needed particularly for when the relation goes away.
	relations map[string]bool
	out       chan []string
	logger    corelogger.Logger
}

// newSubordinateRelationsWatcher creates a watcher that will notify
// about relation lifecycle events for subordinateApp, but filtered to
// be relevant to a unit deployed to a container with the
// principalName app. Global relations will be included, but only
// container-scoped relations for the principal application will be
// emitted - other container-scoped relations will be filtered out.
func newSubordinateRelationsWatcher(
	ctx context.Context,
	backend LegacyBackend,
	applicationService SRWApplicationService,
	relationService SRWRelationService,
	subordinateName, principalName string,
	logger corelogger.Logger) (
	state.StringsWatcher, error,
) {

	w := &subRelationsWatcher{
		backend:            backend,
		applicationService: applicationService,
		relationService:    relationService,
		subordinateName:    subordinateName,
		principalName:      principalName,
		relations:          make(map[string]bool),
		out:                make(chan []string),
		logger:             logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			return w.loop(ctx)
		},
	})
	return w, errors.Capture(err)
}

func (w *subRelationsWatcher) loop(ctx context.Context) error {
	defer close(w.out)
	subordinateAppID, err := w.applicationService.GetApplicationIDByName(ctx, w.principalName)
	if err != nil {
		return errors.Capture(err)
	}

	relationsw, err := w.relationService.WatchLifeSuspendedStatus(ctx, subordinateAppID)
	if err != nil {
		return errors.Capture(err)
	}
	if err := w.catacomb.Add(relationsw); err != nil {
		return errors.Capture(err)
	}
	var (
		sentInitial bool
		out         chan []string

		currentRelations = set.NewStrings()
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case out <- currentRelations.Values():
			sentInitial = true
			currentRelations = set.NewStrings()
			out = nil
		case newRelations, ok := <-relationsw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			for _, relation := range newRelations {
				if currentRelations.Contains(relation) {
					continue
				}
				shouldSend, err := w.shouldSend(ctx, relation)
				if err != nil {
					return errors.Capture(err)
				}
				if shouldSend {
					currentRelations.Add(relation)
				}
			}
			if !sentInitial || currentRelations.Size() > 0 {
				out = w.out
			}
		}
	}
}

func (w *subRelationsWatcher) shouldSend(ctx context.Context, key string) (bool, error) {
	if shouldSend, found := w.relations[key]; found {
		return shouldSend, nil
	}
	result, err := w.shouldSendCheck(ctx, key)
	if err == nil {
		w.relations[key] = result
	}
	return result, errors.Capture(err)
}

func (w *subRelationsWatcher) shouldSendCheck(ctx context.Context, key string) (bool, error) {
	relUUID, err := w.relationService.GetRelationUUIDFromKey(ctx, corerelation.Key(key))
	if errors.Is(err, relationerrors.RelationNotFound) {
		// We never saw it, and it's already gone away, so we can drop it.
		w.logger.Debugf(context.TODO(), "couldn't find unknown relation %q", key)
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	subordinateAppID, err := w.applicationService.GetApplicationIDByName(ctx, w.subordinateName)
	if err != nil {
		return false, errors.Capture(err)
	}
	thisEnd, err := w.relationService.GetRelationEndpoint(ctx, relUUID, subordinateAppID)
	if err != nil {
		return false, errors.Capture(err)
	}
	if thisEnd.Scope == charm.ScopeGlobal {
		return true, nil
	}

	// Only allow container relations if the other end is our
	// principal or the other end is a subordinate.
	otherEnds, err := w.relationService.GetRelatedEndpoints(ctx, relUUID, w.subordinateName)
	if err != nil {
		return false, errors.Capture(err)
	}
	for _, otherEnd := range otherEnds {
		if otherEnd.ApplicationName == w.principalName {
			return true, nil
		}
		otherApp, err := w.backend.Application(otherEnd.ApplicationName)
		if err != nil {
			return false, errors.Capture(err)
		}
		if !otherApp.IsPrincipal() {
			return true, nil
		}
	}
	return false, nil
}

// Changes implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Changes() <-chan []string {
	return w.out
}

// Err implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Err() error {
	return w.catacomb.Err()
}

// Kill implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Stop implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Wait implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Wait() error {
	return w.catacomb.Wait()
}
