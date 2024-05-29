// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/secrets/provider"
)

// WatchableService provides the API for working with the secret service.
type WatchableService struct {
	SecretService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service wrapping the specified state.
func NewWatchableService(
	st State, logger logger.Logger, watcherFactory WatcherFactory, adminConfigGetter BackendAdminConfigGetter,
) *WatchableService {
	return &WatchableService{
		SecretService: SecretService{
			st:                st,
			logger:            logger,
			clock:             clock.WallClock,
			providerGetter:    provider.Provider,
			adminConfigGetter: adminConfigGetter,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchConsumedSecretsChanges watches secrets consumed by the specified unit
// and returns a watcher which notifies of secret URIs that have had a new revision added.
func (s *WatchableService) WatchConsumedSecretsChanges(ctx context.Context, unitName string) (watcher.StringsWatcher, error) {
	tableLocal, queryLocal := s.st.InitialWatchStatementForConsumedSecretsChange(unitName)
	wLocal, err := s.watcherFactory.NewNamespaceWatcher(
		// We are only interested in CREATE changes because
		// the secret_revision.revision is immutable anyway.
		tableLocal, changestream.Create, queryLocal,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	processLocalChanges := func(ctx context.Context, revisionUUIDs ...string) ([]string, error) {
		return s.st.GetConsumedSecretURIsWithChanges(ctx, unitName, revisionUUIDs...)
	}
	sWLocal, err := newSecretWatcher(wLocal, s.logger, processLocalChanges)
	if err != nil {
		return nil, errors.Trace(err)
	}

	tableRemote, queryRemote := s.st.InitialWatchStatementForConsumedRemoteSecretsChange(unitName)
	wRemote, err := s.watcherFactory.NewNamespaceWatcher(
		// We are interested in both CREATE and UPDATE changes on secret_reference table.
		tableRemote, changestream.All, queryRemote,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	processRemoteChanges := func(ctx context.Context, secretIDs ...string) ([]string, error) {
		return s.st.GetConsumedRemoteSecretURIsWithChanges(ctx, unitName, secretIDs...)
	}
	sWRemote, err := newSecretWatcher(wRemote, s.logger, processRemoteChanges)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return eventsource.NewMultiStringsWatcher(ctx, sWLocal, sWRemote)
}

// WatchRemoteConsumedSecretsChanges watches secrets remotely consumed by any unit
// of the specified app and retuens a watcher which notifies of secret URIs
// that have had a new revision added.
func (s *WatchableService) WatchRemoteConsumedSecretsChanges(ctx context.Context, appName string) (watcher.StringsWatcher, error) {
	table, query := s.st.InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(appName)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		table, changestream.All, query,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	processChanges := func(ctx context.Context, secretIDs ...string) ([]string, error) {
		return s.st.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx, appName, secretIDs...)
	}
	return newSecretWatcher(w, s.logger, processChanges)
}

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owned by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *WatchableService) WatchObsolete(ctx context.Context, owners ...CharmSecretOwner) (watcher.StringsWatcher, error) {
	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	table, query := s.st.InitialWatchStatementForObsoleteRevision(appOwners, unitOwners)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		table, changestream.Create, query,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	processChanges := func(ctx context.Context, revisionUUIDs ...string) ([]string, error) {
		return s.st.GetRevisionIDsForObsolete(ctx, appOwners, unitOwners, revisionUUIDs...)
	}
	return newSecretWatcher(w, s.logger, processChanges)
}

// WatchSecretRevisionsExpiryChanges returns a watcher that notifies when the expiry time of a secret revision changes.
func (s *WatchableService) WatchSecretRevisionsExpiryChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	table, query := s.st.InitialWatchStatementForSecretsRevisionExpiryChanges(appOwners, unitOwners)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		table, changestream.All, query,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	processChanges := func(ctx context.Context, revisionUUIDs ...string) ([]watcher.SecretTriggerChange, error) {
		result, err := s.st.GetSecretsRevisionExpiryChanges(ctx, appOwners, unitOwners, revisionUUIDs...)
		if err != nil {
			return nil, errors.Trace(err)
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
	return newSecretWatcher(w, s.logger, processChanges)
}

// WatchSecretsRotationChanges returns a watcher that notifies when the rotation time of a secret changes.
func (s *WatchableService) WatchSecretsRotationChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	table, query := s.st.InitialWatchStatementForSecretsRotationChanges(appOwners, unitOwners)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		table, changestream.All, query,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	processChanges := func(ctx context.Context, secretIDs ...string) ([]watcher.SecretTriggerChange, error) {
		result, err := s.st.GetSecretsRotationChanges(ctx, appOwners, unitOwners, secretIDs...)
		if err != nil {
			return nil, errors.Trace(err)
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
	return newSecretWatcher(w, s.logger, processChanges)
}

func (s *WatchableService) WatchObsoleteUserSecrets(ctx context.Context) (watcher.NotifyWatcher, error) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return watchertest.NewMockNotifyWatcher(ch), nil
}

// WatchSecretBackendChanged notifies when the model secret backend has changed.
func (s *WatchableService) WatchSecretBackendChanged(ctx context.Context) (watcher.NotifyWatcher, error) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return watchertest.NewMockNotifyWatcher(ch), nil
}

// secretWatcher is a watcher that watches for secret changes to a set of strings.
type secretWatcher[T any] struct {
	catacomb catacomb.Catacomb
	logger   logger.Logger

	sourceWatcher watcher.StringsWatcher
	handle        func(ctx context.Context, events ...string) ([]T, error)

	out chan []T
}

func newSecretWatcher[T any](
	sourceWatcher watcher.StringsWatcher, logger logger.Logger,
	handle func(ctx context.Context, events ...string) ([]T, error),
) (*secretWatcher[T], error) {
	w := &secretWatcher[T]{
		sourceWatcher: sourceWatcher,
		logger:        logger,
		handle:        handle,
		out:           make(chan []T),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Trace(err)
}

func (w *secretWatcher[T]) processChanges(events ...string) ([]T, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return w.handle(w.catacomb.Context(ctx), events...)
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
		processed, err := w.processChanges(events.Values()...)
		if err != nil {
			return errors.Trace(err)
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
				return errors.Trace(err)
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
