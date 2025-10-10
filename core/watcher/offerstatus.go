// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/juju/core/status"
)

// OfferStatusChange describes changes to some offer.
type OfferStatusChange struct {
	// UUID is the uuid of the offer.
	UUID string

	// Status is the status of the offer.
	Status status.StatusInfo
}

// OfferStatusChannel is a channel used to notify of changes to
// an offer's status.
// This is deprecated; use <-chan []OfferStatusChange instead.
type OfferStatusChannel = <-chan []OfferStatusChange

// OfferStatusWatcher returns a slice of OfferStatusChanges when an
// offer's status changes.
type OfferStatusWatcher = Watcher[[]OfferStatusChange]
