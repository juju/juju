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
	Revision        int
	NextTriggerTime time.Time
}

func (s SecretTriggerChange) GoString() string {
	revMsg := ""
	if s.Revision > 0 {
		revMsg = fmt.Sprintf("/%d", s.Revision)
	}
	whenMsg := "never"
	if !s.NextTriggerTime.IsZero() {
		interval := s.NextTriggerTime.Sub(time.Now())
		if interval < 0 {
			whenMsg = fmt.Sprintf("%v ago at %s", -interval, s.NextTriggerTime.Format(time.RFC3339))
		} else {
			whenMsg = fmt.Sprintf("in %v at %s", interval, s.NextTriggerTime.Format(time.RFC3339))
		}
	}
	return fmt.Sprintf("%s%s trigger: %s", s.URI.ID, revMsg, whenMsg)
}

// SecretTriggerChannel is a change channel as described in the CoreWatcher docs.
type SecretTriggerChannel <-chan []SecretTriggerChange

// SecretTriggerWatcher conveniently ties a SecretTriggerChannel to the
// worker.Worker that represents its validity.
type SecretTriggerWatcher interface {
	CoreWatcher
	Changes() SecretTriggerChannel
}

// SecretRevisionChange describes changes to a secret.
type SecretRevisionChange struct {
	URI      *secrets.URI
	Revision int
}

func (s SecretRevisionChange) GoString() string {
	return fmt.Sprintf("%s/%d", s.URI.ID, s.Revision)
}

// SecretRevisionChannel is a channel used to notify of
// changes to a secret.
type SecretRevisionChannel <-chan []SecretRevisionChange

// SecretsRevisionWatcher conveniently ties an SecretRevisionChannel to the
// worker.Worker that represents its validity.
type SecretsRevisionWatcher interface {
	CoreWatcher
	Changes() SecretRevisionChannel
}
