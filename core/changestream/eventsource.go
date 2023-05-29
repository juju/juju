// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

// EventSource describes the ability to subscribe
// to a subset of events from a change stream.
type EventSource interface {
	// Subscribe returns a subscription that can receive events from
	// a change stream according to the input subscription options.
	Subscribe(opts ...SubscriptionOption) (Subscription, error)
}
