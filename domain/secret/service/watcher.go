// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/errors"
)

// WatchableService provides the API for working with the secret service.
type WatchableService struct {
	SecretService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service wrapping the specified state.
func NewWatchableService(
	secretState State,
	secretBackendState SecretBackendState,
	leaderEnsurer leadership.Ensurer,
	watcherFactory WatcherFactory,
	logger logger.Logger,
) *WatchableService {
	svc := NewSecretService(secretState, secretBackendState, leaderEnsurer, logger)
	return &WatchableService{
		SecretService:  *svc,
		watcherFactory: watcherFactory,
	}
}

// WatchConsumedSecretsChanges watches secrets consumed by the specified unit
// and returns a watcher which notifies of secret URIs that have had a new revision added.
func (s *WatchableService) WatchConsumedSecretsChanges(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error) {
	tableLocal, queryLocal := s.secretState.InitialWatchStatementForConsumedSecretsChange(unitName)
	wLocal, err := s.watcherFactory.NewNamespaceWatcher(
		// We are only interested in CREATE changes because
		// the secret_revision.revision is immutable anyway.
		queryLocal,
		eventsource.NamespaceFilter(tableLocal, changestream.Changed),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	processLocalChanges := func(ctx context.Context, revisionUUIDs ...string) ([]string, error) {
		return s.secretState.GetConsumedSecretURIsWithChanges(ctx, unitName, revisionUUIDs...)
	}
	sWLocal, err := newSecretStringWatcher(wLocal, s.logger, processLocalChanges)
	if err != nil {
		return nil, errors.Capture(err)
	}

	tableRemote, queryRemote := s.secretState.InitialWatchStatementForConsumedRemoteSecretsChange(unitName)
	wRemote, err := s.watcherFactory.NewNamespaceWatcher(
		// We are interested in both CREATE and UPDATE changes on secret_reference table.
		queryRemote,
		eventsource.NamespaceFilter(tableRemote, changestream.All),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	processRemoteChanges := func(ctx context.Context, secretIDs ...string) ([]string, error) {
		return s.secretState.GetConsumedRemoteSecretURIsWithChanges(ctx, unitName, secretIDs...)
	}
	sWRemote, err := newSecretStringWatcher(wRemote, s.logger, processRemoteChanges)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return eventsource.NewMultiStringsWatcher(ctx, sWLocal, sWRemote)
}

// WatchRemoteConsumedSecretsChanges watches secrets remotely consumed by any unit
// of the specified app and retuens a watcher which notifies of secret URIs
// that have had a new revision added.
func (s *WatchableService) WatchRemoteConsumedSecretsChanges(_ context.Context, appName string) (watcher.StringsWatcher, error) {
	table, query := s.secretState.InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(appName)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		query,
		eventsource.NamespaceFilter(table, changestream.All),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	processChanges := func(ctx context.Context, secretIDs ...string) ([]string, error) {
		return s.secretState.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx, appName, secretIDs...)
	}
	return newSecretStringWatcher(w, s.logger, processChanges)
}

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owned by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *WatchableService) WatchObsolete(_ context.Context, owners ...CharmSecretOwner) (watcher.StringsWatcher, error) {
	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}
	return newObsoleteWatcher(s.secretState, s.watcherFactory, s.logger, owners...)
}

// WatchSecretRevisionsExpiryChanges returns a watcher that notifies when the expiry time of a secret revision changes.
func (s *WatchableService) WatchSecretRevisionsExpiryChanges(_ context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	table, query := s.secretState.InitialWatchStatementForSecretsRevisionExpiryChanges(appOwners, unitOwners)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		query,
		eventsource.NamespaceFilter(table, changestream.All),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	processChanges := func(ctx context.Context, revisionUUIDs ...string) ([]watcher.SecretTriggerChange, error) {
		result, err := s.secretState.GetSecretsRevisionExpiryChanges(ctx, appOwners, unitOwners, revisionUUIDs...)
		if err != nil {
			return nil, errors.Capture(err)
		}
		changes := make([]watcher.SecretTriggerChange, len(result))
		for i, r := range result {
			changes[i] = watcher.SecretTriggerChange{
				URI:             r.URI,
				Revision:        r.Revision,
				NextTriggerTime: r.NextTriggerTime,
			}
		}
		return changes, nil
	}
	return newSecretStringWatcher(w, s.logger, processChanges)
}

type obsoleteWatcher struct {
	catacomb       catacomb.Catacomb
	logger         logger.Logger
	state          State
	watcherFactory WatcherFactory

	appOwners  domainsecret.ApplicationOwners
	unitOwners domainsecret.UnitOwners

	// sourceWatcher watches for both `All` secret events and obsolete revision `Changed` changes.
	// So the events are secret URIs and obsolete revision UUIDs.
	sourceWatcher watcher.StringsWatcher

	// knownSecretURIs is a set of currently owned secret URIs.
	// Tracking this set allows us to identify if a deletion event corresponds to a previously owned secret.
	// When a deletion event is received, the secret data is no longer available in the database,
	// so we cannot query the database to determine if the secret was previously owned.
	knownSecretURIs set.Strings

	out chan []string
}

func newObsoleteWatcher(
	state State,
	watcherFactory WatcherFactory,
	logger logger.Logger,
	owners ...CharmSecretOwner,
) (*obsoleteWatcher, error) {
	if len(owners) == 0 {
		return nil, errors.Errorf("at least one owner must be provided for the obsolete secret watcher")
	}

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	w := &obsoleteWatcher{
		logger:          logger,
		state:           state,
		watcherFactory:  watcherFactory,
		appOwners:       appOwners,
		unitOwners:      unitOwners,
		knownSecretURIs: set.NewStrings(),
		out:             make(chan []string),
	}

	if err := w.init(); err != nil {
		return nil, errors.Errorf("initialising the obsolete secret watcher: %w", err)
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *obsoleteWatcher) init() error {
	tableSecrets, querySecrets := w.state.InitialWatchStatementForOwnedSecrets(w.appOwners, w.unitOwners)
	tableObsoleteRevisions, queryObsoleteRevisions := w.state.InitialWatchStatementForObsoleteRevision(
		w.appOwners, w.unitOwners,
	)

	initialQuery := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		var initials []string
		// Get the initial secret changes.
		secretChanges, err := querySecrets(ctx, runner)
		if err != nil {
			return nil, errors.Capture(err)
		}
		initials = append(initials, secretChanges...)

		// Get the initial obsolete revision changes.
		obsoleteRevisionChanges, err := queryObsoleteRevisions(ctx, runner)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return append(initials, obsoleteRevisionChanges...), nil
	}

	var err error
	w.sourceWatcher, err = w.watcherFactory.NewNamespaceWatcher(
		initialQuery,
		eventsource.NamespaceFilter(tableSecrets, changestream.All),
		eventsource.NamespaceFilter(tableObsoleteRevisions, changestream.Changed),
	)
	return errors.Capture(err)
}

func (w *obsoleteWatcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func splitSecretRevision(s string) (string, int) {
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return parts[0], 0
	}
	rev, _ := strconv.Atoi(parts[1])
	return parts[0], rev
}

func (w *obsoleteWatcher) mergeSecretChanges(
	ctx context.Context, currentChanges []string, receivcedSecretIDs []string,
) ([]string, error) {
	if len(receivcedSecretIDs) == 0 {
		return currentChanges, nil
	}

	// pushChanges pushes the secret ID to the changes slice.
	// At the same time, any previously added obsolete revisions of this secret are removed from the slice.
	pushChanges := func(secretID string) {
		currentChanges = slices.DeleteFunc(currentChanges, func(s string) bool {
			id, _ := splitSecretRevision(s)
			return id == secretID
		})
		currentChanges = append(currentChanges, secretID)
	}

	for _, uriStr := range receivcedSecretIDs {
		uri, err := coresecrets.ParseURI(uriStr)
		if err != nil {
			return currentChanges, errors.Capture(err)
		}
		owned, err := w.state.IsSecretOwnedBy(ctx, uri, w.appOwners, w.unitOwners)
		if err != nil {
			return currentChanges, errors.Capture(err)
		}
		if owned {
			// It's still owned, so the event must be triggered by an update.
			// Ensure we are tracking the secret URI.
			w.knownSecretURIs.Add(uri.ID)

			// We are only interested in a previously owned secret that has been deleted,
			// so ignore this one and continue.
			continue
		}

		if w.knownSecretURIs.Contains(uri.ID) {
			// An onwed secret has been deleted, we need to notify the URI change.
			pushChanges(uri.ID)

			// No need to track this one anymore.
			w.knownSecretURIs.Remove(uri.ID)
		}
	}
	return currentChanges, nil
}

func (w *obsoleteWatcher) mergeRevisionChanges(
	ctx context.Context, currentChanges []string, newChanges []string,
) ([]string, error) {
	if len(newChanges) == 0 {
		return currentChanges, nil
	}

	// We are receiving all the obsolete revision UUIDs changes in the model, so we need to filter
	// only the ones that are owned.
	revisionIDs, err := w.state.GetRevisionIDsForObsolete(
		ctx, w.appOwners, w.unitOwners, newChanges...,
	)
	if err != nil {
		return currentChanges, errors.Capture(err)
	}

	return append(currentChanges, revisionIDs...), nil
}

func (w *obsoleteWatcher) loop() error {
	defer close(w.out)

	ctx, cancel := w.scopedContext()
	defer cancel()
	// To allow the initial event to be sent.
	out := w.out

	splitEvents := func(events []string) (secretEvents, revisionEvents []string) {
		if len(events) == 0 {
			return
		}

		// The source watcher may emit events from secret_metadata and secret_revision_obsolete tables.
		// We need to split the events into secret URI strings and revision UUIDs strings.
		for _, e := range events {
			if _, err := coresecrets.ParseURI(e); err == nil {
				secretEvents = append(secretEvents, e)
				continue
			}
			revisionEvents = append(revisionEvents, e)
		}
		return
	}

	var changes []string
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("source watcher closed")
			}
			secretEvents, revisionEvents := splitEvents(events)
			var err error
			changes, err = w.mergeSecretChanges(ctx, changes, secretEvents)
			if err != nil {
				return errors.Capture(err)
			}
			changes, err = w.mergeRevisionChanges(ctx, changes, revisionEvents)
			if err != nil {
				return errors.Capture(err)
			}
		case out <- changes:
			changes = nil
			out = nil
		}

		if len(changes) > 0 {
			// We have changes, so we need to notify the changes.
			out = w.out
		}
	}
}

// Changes returns the channel of secret changes.
func (w *obsoleteWatcher) Changes() <-chan []string {
	return w.out
}

// Stop stops the watcher.
func (w *obsoleteWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher.
func (w *obsoleteWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher to die,
// and returns the error with which it was killed.
func (w *obsoleteWatcher) Wait() error {
	return w.catacomb.Wait()
}

// WatchSecretsRotationChanges returns a watcher that notifies when the rotation time of a secret changes.
func (s *WatchableService) WatchSecretsRotationChanges(_ context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	table, query := s.secretState.InitialWatchStatementForSecretsRotationChanges(appOwners, unitOwners)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		query,
		eventsource.NamespaceFilter(table, changestream.All),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	processChanges := func(ctx context.Context, secretIDs ...string) ([]watcher.SecretTriggerChange, error) {
		result, err := s.secretState.GetSecretsRotationChanges(ctx, appOwners, unitOwners, secretIDs...)
		if err != nil {
			return nil, errors.Capture(err)
		}
		changes := make([]watcher.SecretTriggerChange, len(result))
		for i, r := range result {
			changes[i] = watcher.SecretTriggerChange{
				URI:             r.URI,
				Revision:        r.Revision,
				NextTriggerTime: r.NextTriggerTime,
			}
		}
		return changes, nil
	}
	return newSecretStringWatcher(w, s.logger, processChanges)
}

// WatchObsoleteUserSecretsToPrune returns a watcher that notifies when a user secret revision is obsolete and ready to be pruned.
func (s *WatchableService) WatchObsoleteUserSecretsToPrune(ctx context.Context) (watcher.NotifyWatcher, error) {
	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		if len(changes) == 0 {
			return nil, nil
		}
		obsoleteRevs, err := s.secretState.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if len(obsoleteRevs) == 0 {
			return nil, nil
		}
		// We merge the changes to one event to avoid multiple events.
		// Because the prune worker will prune all obsolete revisions once.
		return changes[:1], nil
	}

	wObsolete, err := s.watcherFactory.NewNotifyMapperWatcher(
		mapper,
		eventsource.NamespaceFilter(
			s.secretState.NamespaceForWatchSecretRevisionObsolete(),
			changestream.Changed,
		),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	wAutoPrune, err := s.watcherFactory.NewNotifyMapperWatcher(
		mapper,
		eventsource.NamespaceFilter(
			s.secretState.NamespaceForWatchSecretMetadata(),
			changestream.Changed,
		),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return eventsource.NewMultiNotifyWatcher(ctx, wObsolete, wAutoPrune)
}

// secretWatcher is a watcher that watches for secret changes to a set of strings.
type secretWatcher[T any] struct {
	catacomb catacomb.Catacomb
	logger   logger.Logger

	sourceWatcher  watcher.StringsWatcher
	processChanges func(ctx context.Context, events ...string) ([]T, error)

	out chan []T
}

func newSecretStringWatcher[T any](
	sourceWatcher watcher.StringsWatcher, logger logger.Logger,
	processChanges func(ctx context.Context, events ...string) ([]T, error),
) (*secretWatcher[T], error) {
	w := &secretWatcher[T]{
		sourceWatcher:  sourceWatcher,
		logger:         logger,
		processChanges: processChanges,
		out:            make(chan []T),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *secretWatcher[T]) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *secretWatcher[T]) loop() error {
	defer close(w.out)

	var (
		historyIDs set.Strings
		changes    []T
	)
	// To allow the initial event to be sent.
	out := w.out
	addChanges := func(events set.Strings) error {
		if len(events) == 0 {
			return nil
		}
		ctx, cancel := w.scopedContext()
		processed, err := w.processChanges(ctx, events.Values()...)
		cancel()
		if err != nil {
			return errors.Capture(err)
		}
		if len(processed) == 0 {
			return nil
		}

		if historyIDs == nil {
			historyIDs = set.NewStrings()
		}
		for _, v := range processed {
			id := fmt.Sprint(v)
			if historyIDs.Contains(id) {
				continue
			}
			changes = append(changes, v)
			historyIDs.Add(id)
		}

		out = w.out
		return nil
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			if err := addChanges(set.NewStrings(events...)); err != nil {
				return errors.Capture(err)
			}
		case out <- changes:
			changes = nil
			out = nil
		}
	}
}

// Changes returns the channel of secret changes.
func (w *secretWatcher[T]) Changes() <-chan []T {
	return w.out
}

// Stop stops the watcher.
func (w *secretWatcher[T]) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher via its tomb.
func (w *secretWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *secretWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}
