// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/juju/core/status"
)

// OfferStatusChange describes changes to some offer.
type OfferStatusChange struct {
	// Name is the name of the offer.
	Name string

	// Status is the status of the offer.
	Status status.StatusInfo
}

// OfferStatusChannel is a channel used to notify of changes to
// an offer's status.
type OfferStatusChannel <-chan []OfferStatusChange

// OfferStatusWatcher conveniently ties an OfferStatusChannel to the
// worker.Worker that represents its validity.
type OfferStatusWatcher interface {
	CoreWatcher
	Changes() OfferStatusChannel
}
