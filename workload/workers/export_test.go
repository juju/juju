// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/workload"
)

func ExposeChannel(events *EventHandlers) chan []workload.Event {
	return events.events
}
