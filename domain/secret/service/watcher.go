// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/secrets/provider"
)

// WatchableService provides the API for working with the secret service.
type WatchableService struct {
	SecretService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service wrapping the specified state.
func NewWatchableService(
	st State, logger Logger, watcherFactory WatcherFactory, adminConfigGetter BackendAdminConfigGetter,
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
	ch := make(chan []string, 1)
	ch <- []string{}
	return watchertest.NewMockStringsWatcher(ch), nil
}

// WatchRemoteConsumedSecretsChanges watches secrets remotely consumed by any unit
// of the specified app and retuens a watcher which notifies of secret URIs
// that have had a new revision added.
func (s *WatchableService) WatchRemoteConsumedSecretsChanges(ctx context.Context, appName string) (watcher.StringsWatcher, error) {
	ch := make(chan []string, 1)
	ch <- []string{}
	return watchertest.NewMockStringsWatcher(ch), nil
}

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owned by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *WatchableService) WatchObsolete(ctx context.Context, owners ...CharmSecretOwner) (watcher.StringsWatcher, error) {
	// TODO: do we allow you watch for all obsolete secrets if no owners are provided??
	if len(owners) == 0 {
		return nil, errors.New("at least one owner must be provided")
	}

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	table, stmt := s.st.InitialWatchStatementForObsoleteRevision(ctx, appOwners, unitOwners)
	w, err := s.watcherFactory.NewNamespaceWatcher(
		table,
		changestream.Update,
		stmt,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newObsoleteRevisionWatcher(appOwners, unitOwners, w, s.logger, s.st.GetRevisionIDsForObsolete), nil
}

type obsoleteRevisionWatcher struct {
	tomb   tomb.Tomb
	logger Logger

	appOwners      domainsecret.ApplicationOwners
	unitOwners     domainsecret.UnitOwners
	sourceWatcher  watcher.StringsWatcher
	processChanges func(
		ctx context.Context,
		appOwners domainsecret.ApplicationOwners,
		unitOwners domainsecret.UnitOwners,
		revisionUUID ...string,
	) ([]string, error)

	out chan []string
}

func newObsoleteRevisionWatcher(
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	sourceWatcher watcher.StringsWatcher, logger Logger,
	processChanges func(
		ctx context.Context,
		appOwners domainsecret.ApplicationOwners,
		unitOwners domainsecret.UnitOwners,
		revisionUUID ...string,
	) ([]string, error),
) *obsoleteRevisionWatcher {
	w := &obsoleteRevisionWatcher{
		appOwners:      appOwners,
		unitOwners:     unitOwners,
		sourceWatcher:  sourceWatcher,
		logger:         logger,
		processChanges: processChanges,
		out:            make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *obsoleteRevisionWatcher) loop() (err error) {
	var changes []string
	ctx := w.tomb.Context(context.Background())

	out := w.out
	// Get the initial set of changes.
	changes, err = w.processChanges(ctx, w.appOwners, w.unitOwners)
	if err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case revisionUUIDs, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			w.logger.Debugf("received obsolete revision changes: %v", revisionUUIDs)

			var err error
			changes, err = w.processChanges(ctx, w.appOwners, w.unitOwners, revisionUUIDs...)
			if err != nil {
				return errors.Trace(err)
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
		}
	}
}

// Changes (watcher.StringsWatcher) returns the channel of obsolete secret changes.
func (w *obsoleteRevisionWatcher) Changes() <-chan []string {
	return w.out
}

// Kill (worker.Worker) kills the watcher via its tomb.
func (w *obsoleteRevisionWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *obsoleteRevisionWatcher) Wait() error {
	return w.tomb.Wait()
}

// TODO(secrets) - replace with real watcher
func newMockTriggerWatcher(ch watcher.SecretTriggerChannel) *mockSecretTriggerWatcher {
	w := &mockSecretTriggerWatcher{ch: ch}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

type mockSecretTriggerWatcher struct {
	tomb tomb.Tomb
	ch   watcher.SecretTriggerChannel
}

func (w *mockSecretTriggerWatcher) Changes() watcher.SecretTriggerChannel {
	return w.ch
}

func (w *mockSecretTriggerWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *mockSecretTriggerWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *mockSecretTriggerWatcher) Err() error {
	return w.tomb.Err()
}

func (w *mockSecretTriggerWatcher) Wait() error {
	return w.tomb.Wait()
}

func (s *WatchableService) WatchSecretRevisionsExpiryChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	ch := make(chan []watcher.SecretTriggerChange, 1)
	ch <- []watcher.SecretTriggerChange{}
	return newMockTriggerWatcher(ch), nil
}

func (s *WatchableService) WatchSecretsRotationChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	ch := make(chan []watcher.SecretTriggerChange, 1)
	ch <- []watcher.SecretTriggerChange{}
	return newMockTriggerWatcher(ch), nil
}

func (s *WatchableService) WatchObsoleteUserSecrets(ctx context.Context) (watcher.NotifyWatcher, error) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return watchertest.NewMockNotifyWatcher(ch), nil
}

func (s *WatchableService) SecretRotated(ctx context.Context, uri *secrets.URI, originalRev int, skip bool) error {
	md, err := s.GetSecret(ctx, uri)
	if err != nil {
		return errors.Trace(err)
	}
	if !md.RotatePolicy.WillRotate() {
		s.logger.Debugf("secret %q was rotated but now is set to not rotate")
		return nil
	}
	lastRotateTime := md.NextRotateTime
	if lastRotateTime == nil {
		now := s.clock.Now()
		lastRotateTime = &now
	}
	nextRotateTime := *md.RotatePolicy.NextRotateTime(*lastRotateTime)
	s.logger.Debugf("secret %q was rotated: rev was %d, now %d", uri.ID, originalRev, md.LatestRevision)
	// If the secret will expire before it is due to be next rotated, rotate sooner to allow
	// the charm a chance to update it before it expires.
	willExpire := md.LatestExpireTime != nil && md.LatestExpireTime.Before(nextRotateTime)
	forcedRotateTime := lastRotateTime.Add(secrets.RotateRetryDelay)
	if willExpire {
		s.logger.Warningf("secret %q rev %d will expire before next scheduled rotation", uri.ID, md.LatestRevision)
	}
	if willExpire && forcedRotateTime.Before(*md.LatestExpireTime) || !skip && md.LatestRevision == originalRev {
		nextRotateTime = forcedRotateTime
	}
	s.logger.Debugf("secret %q next rotate time is now: %s", uri.ID, nextRotateTime.UTC().Format(time.RFC3339))

	// TODO(secrets)
	return nil
}
