// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// SecretRotationChange describes changes to secret rotation config.
type SecretRotationChange struct {
	URI            *secrets.URI
	RotateInterval time.Duration
	LastRotateTime time.Time
}

// SecretRotationChannel is a change channel as described in the CoreWatcher docs.
type SecretRotationChannel <-chan []SecretRotationChange

// SecretRotationWatcher conveniently ties a SecretRotationChannel to the
// worker.Worker that represents its validity.
type SecretRotationWatcher interface {
	CoreWatcher
	Changes() SecretRotationChannel
}
