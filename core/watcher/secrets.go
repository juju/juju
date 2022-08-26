// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// SecretTriggerChange describes changes to a secret trigger.
// eg rotation or expiry.
type SecretTriggerChange struct {
	URI             *secrets.URI
	NextTriggerTime time.Time
}

// SecretTriggerChannel is a change channel as described in the CoreWatcher docs.
type SecretTriggerChannel <-chan []SecretTriggerChange

// SecretTriggerWatcher conveniently ties a SecretTriggerChannel to the
// worker.Worker that represents its validity.
type SecretTriggerWatcher interface {
	CoreWatcher
	Changes() SecretTriggerChannel
}
