// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"fmt"
	"time"

	"github.com/juju/juju/core/secrets"
)

// SecretTriggerChange describes changes to a secret trigger.
// eg rotation or expiry.
type SecretTriggerChange struct {
	URI             *secrets.URI
	NextTriggerTime time.Time
}

func (s SecretTriggerChange) GoString() string {
	return fmt.Sprintf("%s trigger: in %v at %s", s.URI.ID, s.NextTriggerTime.Sub(time.Now()), s.NextTriggerTime.Format(time.RFC3339))
}

// SecretTriggerChannel is a change channel as described in the CoreWatcher docs.
type SecretTriggerChannel <-chan []SecretTriggerChange

// SecretTriggerWatcher conveniently ties a SecretTriggerChannel to the
// worker.Worker that represents its validity.
type SecretTriggerWatcher interface {
	CoreWatcher
	Changes() SecretTriggerChannel
}
