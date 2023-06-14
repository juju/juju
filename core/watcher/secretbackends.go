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

// SecretBackendRotateChannel is a change channel as described in the
// CoreWatcher docs.
// This is deprecated; use <-chan []SecretBackendRotateChange instead.
type SecretBackendRotateChannel = <-chan []SecretBackendRotateChange

// SecretBackendRotateWatcher represents a watcher that returns a slice of
// SecretBackendRotateChange.
type SecretBackendRotateWatcher = Watcher[[]SecretBackendRotateChange]
