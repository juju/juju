// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
)

// WatchConsumedSecretsChanges watches secrets consumed by the specified unit
// and returns a watcher which notifies of secret URIs that have had a new revision added.
func (s *SecretService) WatchConsumedSecretsChanges(ctx context.Context, unitName string) (watcher.StringsWatcher, error) {
	ch := make(chan []string, 1)
	ch <- []string{}
	return watchertest.NewMockStringsWatcher(ch), nil
}

// WatchRemoteConsumedSecretsChanges watches secrets remotely consumed by any unit
// of the specified app and retuens a watcher which notifies of secret URIs
// that have had a new revision added.
func (s *SecretService) WatchRemoteConsumedSecretsChanges(ctx context.Context, appName string) (watcher.StringsWatcher, error) {
	ch := make(chan []string, 1)
	ch <- []string{}
	return watchertest.NewMockStringsWatcher(ch), nil
}

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owed by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *SecretService) WatchObsolete(ctx context.Context, owners ...CharmSecretOwner) (watcher.StringsWatcher, error) {
	ch := make(chan []string, 1)
	ch <- []string{}
	return watchertest.NewMockStringsWatcher(ch), nil
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

func (s *SecretService) WatchSecretRevisionsExpiryChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	ch := make(chan []watcher.SecretTriggerChange, 1)
	ch <- []watcher.SecretTriggerChange{}
	return newMockTriggerWatcher(ch), nil
}

func (s *SecretService) WatchSecretsRotationChanges(ctx context.Context, owners ...CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	ch := make(chan []watcher.SecretTriggerChange, 1)
	ch <- []watcher.SecretTriggerChange{}
	return newMockTriggerWatcher(ch), nil
}

func (s *SecretService) WatchObsoleteUserSecrets(ctx context.Context) (watcher.NotifyWatcher, error) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return watchertest.NewMockNotifyWatcher(ch), nil
}

func (s *SecretService) SecretRotated(ctx context.Context, uri *secrets.URI, originalRev int, skip bool) error {
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
