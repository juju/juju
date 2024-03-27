// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
)

func (s *SecretService) WatchConsumedSecretsChanges(ctx context.Context, consumer SecretConsumer) (watcher.StringsWatcher, error) {
	return watchertest.NewMockStringsWatcher(make(chan []string)), nil
}

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owed by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *SecretService) WatchObsolete(ctx context.Context, owner CharmSecretOwner) (watcher.StringsWatcher, error) {
	return watchertest.NewMockStringsWatcher(make(chan []string)), nil
}

func (s *SecretService) WatchSecretRevisionsExpiryChanges(ctx context.Context, owner CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	panic("implement me")
}

func (s *SecretService) WatchSecretsRotationChanges(ctx context.Context, owner CharmSecretOwner) (watcher.SecretTriggerWatcher, error) {
	panic("implement me")
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

	panic("implement me")
}
