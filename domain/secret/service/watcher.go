// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
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

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	table, query := s.secretState.InitialWatchStatementForObsoleteRevision(appOwners, unitOwners)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		query,
		eventsource.NamespaceFilter(table, changestream.Changed),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	processChanges := func(ctx context.Context, revisionUUIDs ...string) ([]string, error) {
		return s.secretState.GetRevisionIDsForObsolete(ctx, appOwners, unitOwners, revisionUUIDs...)
	}
	return newSecretStringWatcher(w, s.logger, processChanges)
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
	mapper := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
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

	wObsolete, err := s.watcherFactory.NewNamespaceNotifyMapperWatcher(
		s.secretState.NamespaceForWatchSecretRevisionObsolete(), changestream.Changed, mapper,
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	wAutoPrune, err := s.watcherFactory.NewNamespaceNotifyMapperWatcher(
		s.secretState.NamespaceForWatchSecretMetadata(), changestream.Changed, mapper,
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
