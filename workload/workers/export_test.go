// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/workload"
)

func ExposeEventHandlers(eh *EventHandlers) *eventHandlersData {
	return &eh.data
}

func ExposeEvents(e *Events) chan []workload.Event {
	return e.events
}
