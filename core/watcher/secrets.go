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

// String returns a string representation of the change.
func (s SecretTriggerChange) String() string {
	str := s.URI.String()
	if s.Revision > 0 {
		str = fmt.Sprintf("%s/%d", s.URI.String(), s.Revision)
	}
	return str
}

// GoString returns a Go-syntax representation of the change.
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

// SecretTriggerChannel is a change channel as described in the CoreWatcher
// docs.
// This is deprecated; use <-chan []SecretTriggerChange instead.
type SecretTriggerChannel = <-chan []SecretTriggerChange

// SecretTriggerWatcher represents a watcher that reports the latest
// trigger of a secret.
type SecretTriggerWatcher = Watcher[[]SecretTriggerChange]

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
// This is deprecated; use <-chan []SecretRevisionChange instead.
type SecretRevisionChannel = <-chan []SecretRevisionChange

// SecretsRevisionWatcher represents a watcher that reports the latest
// revision of a secret.
type SecretsRevisionWatcher = Watcher[[]SecretRevisionChange]
