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
	"github.com/juju/collections/transform"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/secret"
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
func (s *WatchableService) WatchRemoteConsumedSecretsChanges(ctx context.Context, appName string) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
func (s *WatchableService) WatchObsolete(ctx context.Context, owners ...CharmSecretOwner) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}
	appOwners, unitOwners := splitCharmSecretOwners(owners...)

	tableSecrets, querySecrets := s.secretState.InitialWatchStatementForOwnedSecrets(appOwners, unitOwners)
	tableObsoleteRevisions, queryObsoleteRevisions := s.secretState.InitialWatchStatementForObsoleteRevision(
		appOwners, unitOwners,
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

	return s.watcherFactory.NewNamespaceMapperWatcher(
		initialQuery,
		obsoleteWatcherMapperFunc(
			s.logger,
			s.secretState,
			appOwners, unitOwners,
			tableSecrets, tableObsoleteRevisions,
		),
		eventsource.NamespaceFilter(tableSecrets, changestream.All),
		eventsource.NamespaceFilter(tableObsoleteRevisions, changestream.Changed),
	)
}

func obsoleteWatcherMapperFunc(
	logger logger.Logger,
	state State,
	appOwners secret.ApplicationOwners,
	unitOwners secret.UnitOwners,
	tableSecrets, tableObsoleteRevisions string,
) eventsource.Mapper {
	// knownSecretURIs is a set of currently owned secret URIs.
	// Tracking this set allows us to identify if a deletion event corresponds to a previously owned secret.
	// When a deletion event is received, the secret data is no longer available in the database,
	// so we cannot query the database to determine if the secret was previously owned.
	knownSecretURIs := set.NewStrings()

	mergeSecretChange := func(
		currentChanges []changestream.ChangeEvent,
		currentOwnedSecretIDs set.Strings,
		secretChange changestream.ChangeEvent,
	) ([]changestream.ChangeEvent, error) {
		// pushChanges pushes the secret ID to the changes slice.
		// At the same time, any previously added obsolete revisions of this secret are removed from the slice.
		pushChanges := func(change changestream.ChangeEvent) {
			currentChanges = slices.DeleteFunc(currentChanges, func(c changestream.ChangeEvent) bool {
				id, _ := splitSecretRevision(c.Changed())
				return id == change.Changed()
			})
			currentChanges = append(currentChanges, change)
		}

		secretChangeID := secretChange.Changed()
		if currentOwnedSecretIDs.Contains(secretChangeID) {
			// It's still owned, so the event must be triggered by an update.
			// Ensure we are tracking the secret URI.
			knownSecretURIs.Add(secretChangeID)

			// We are only interested in a previously owned secret that has been deleted,
			// so ignore this one.
			return currentChanges, nil
		}

		if knownSecretURIs.Contains(secretChangeID) {
			// An owned secret has been deleted, we need to notify the URI change.
			pushChanges(secretChange)

			// No need to track this one anymore.
			knownSecretURIs.Remove(secretChangeID)
		}
		return currentChanges, nil
	}

	mergeRevisionChange := func(
		currentChanges []changestream.ChangeEvent,
		revisionUUIDAndIDMap map[string]string,
		revisionChange changestream.ChangeEvent,
	) ([]changestream.ChangeEvent, error) {
		// We are receiving all the obsolete revision UUIDs changes in the model, so we need to filter
		// only the one that is owned.
		if revisionID, ok := revisionUUIDAndIDMap[revisionChange.Changed()]; ok {
			currentChanges = append(currentChanges, newMaskedChangeIDEvent(revisionChange, revisionID))
		}
		return currentChanges, nil
	}

	splitEvents := func(events []changestream.ChangeEvent) (secretEventValues, revisionEventValues []string) {
		if len(events) == 0 {
			return
		}

		// The source watcher may emit events from secret_metadata and secret_revision_obsolete tables.
		// We need to split the events into secret URI strings and revision UUIDs strings.
		for _, e := range events {
			if _, err := coresecrets.ParseURI(e.Changed()); err == nil {
				secretEventValues = append(secretEventValues, e.Changed())
				continue
			}
			revisionEventValues = append(revisionEventValues, e.Changed())
		}
		return
	}

	return func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		if len(changes) == 0 {
			return nil, nil
		}

		var result []changestream.ChangeEvent
		var err error

		secretEventValues, revisionEventValues := splitEvents(changes)

		// We fetch current owned secret IDs and revision UUIDs once
		// per batch of changes, to avoid multiple queries for each
		// change event. This is more efficient than querying the
		// database for each change event. We ensure that we have the
		// latest owned secrets and revisions but may miss the database
		// changes that happen during the processing of the changes.
		// This is acceptable because the source watcher will emit the
		// changes again for these changes.
		var currentOwnedSecretIDs set.Strings
		var revisionUUIDAndIDMap map[string]string
		if len(secretEventValues) > 0 {
			ownedSecretIDs, err := state.GetOwnedSecretIDs(ctx, appOwners, unitOwners)
			if err != nil {
				return nil, errors.Capture(err)
			}
			currentOwnedSecretIDs = set.NewStrings(ownedSecretIDs...)
		}
		if len(revisionEventValues) > 0 {
			revisionUUIDAndIDMap, err = state.GetRevisionIDsForObsolete(
				ctx, appOwners, unitOwners, revisionEventValues...,
			)
			if err != nil {
				return nil, errors.Capture(err)
			}
		}

		// The source watcher may emit events from secret_metadata
		// and secret_revision_obsolete tables.
		for _, change := range changes {
			switch change.Namespace() {
			case tableSecrets:
				result, err = mergeSecretChange(result, currentOwnedSecretIDs, change)
			case tableObsoleteRevisions:
				result, err = mergeRevisionChange(result, revisionUUIDAndIDMap, change)
			default:
				// This should never happen, but just in case.
				// We are not interested in any other events.
				logger.Warningf(ctx, "unknown event with namespace: %q received in obsolete watcher", change.Namespace())
			}
			if err != nil {
				return nil, errors.Errorf(
					"processing change event %s/%s in obsolete watcher mapper: %v",
					change.Namespace(), change.Changed(), err,
				)
			}
		}
		return transform.Slice(result, func(c changestream.ChangeEvent) string {
			return c.Changed()
		}), nil
	}
}

func splitSecretRevision(s string) (string, int) {
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return parts[0], 0
	}
	rev, _ := strconv.Atoi(parts[1])
	return parts[0], rev
}

type maskedChangeIDEvent struct {
	changestream.ChangeEvent
	id string
}

func newMaskedChangeIDEvent(change changestream.ChangeEvent, id string) changestream.ChangeEvent {
	return maskedChangeIDEvent{
		ChangeEvent: change,
		id:          id,
	}
}

func (m maskedChangeIDEvent) Changed() string {
	return m.id
}

// WatchSecretRevisionsExpiryChanges returns a watcher that notifies when the expiry time of a secret revision changes.
func (s *WatchableService) WatchSecretRevisionsExpiryChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
func (s *WatchableService) WatchSecretsRotationChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
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
		return transform.Slice(changes[:1], func(c changestream.ChangeEvent) string {
			return c.Changed()
		}), nil
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
		Name: "secret-watcher",
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
