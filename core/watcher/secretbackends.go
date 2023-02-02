// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"fmt"
	"time"
)

// SecretBackendRotateChange describes changes to a secret backend
// rotation trigger.
type SecretBackendRotateChange struct {
	ID              string
	Name            string
	NextTriggerTime time.Time
}

func (s SecretBackendRotateChange) GoString() string {
	whenMsg := "never"
	if !s.NextTriggerTime.IsZero() {
		interval := s.NextTriggerTime.Sub(time.Now())
		if interval < 0 {
			whenMsg = fmt.Sprintf("%v ago at %s", -interval, s.NextTriggerTime.Format(time.RFC3339))
		} else {
			whenMsg = fmt.Sprintf("in %v at %s", interval, s.NextTriggerTime.Format(time.RFC3339))
		}
	}
	return fmt.Sprintf("%s token rotate: %s", s.Name, whenMsg)
}

// SecretBackendRotateChannel is a change channel as described in the CoreWatcher docs.
type SecretBackendRotateChannel <-chan []SecretBackendRotateChange

// SecretBackendRotateWatcher conveniently ties a SecretBackendRotateChannel to the
// worker.Worker that represents its validity.
type SecretBackendRotateWatcher interface {
	CoreWatcher
	Changes() SecretBackendRotateChannel
}
