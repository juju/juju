// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import "github.com/juju/juju/core/changestream"

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// TODO(wallyworld) - remove when we have dqlite watchers on k8s
// noopSubscription doesn't provide any events - used on k8s
// because dqlite watchers not supported
type noopSubscription struct{}

func (noopSubscription) Changes() <-chan []changestream.ChangeEvent {
	return make(<-chan []changestream.ChangeEvent)
}

func (noopSubscription) Unsubscribe() {}

func (noopSubscription) Done() <-chan struct{} {
	return make(<-chan struct{})
}
