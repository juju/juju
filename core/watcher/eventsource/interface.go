// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"github.com/juju/juju/core/changestream"
)

// EventQueue describes the ability to subscribe
// to a subset of events from a change stream.
type EventQueue interface {
	// Subscribe returns a subscription that can receive events from
	// a change stream according to the input subscription options.
	Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error)
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}
