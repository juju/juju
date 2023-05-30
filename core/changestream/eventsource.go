// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import "github.com/juju/juju/core/database"

// EventSource describes the ability to subscribe
// to a subset of events from a change stream.
type EventSource interface {
	// Subscribe returns a subscription that can receive events from
	// a change stream according to the input subscription options.
	Subscribe(opts ...SubscriptionOption) (Subscription, error)
}

// WatchableDB describes the ability to run transactions against a database
// and to subscribe to events emitted from that same source.
type WatchableDB interface {
	database.TxnRunner
	EventSource
}
