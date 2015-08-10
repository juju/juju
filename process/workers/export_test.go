// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/process"
)

func ExposeChannel(events *EventHandler) chan []process.Event {
	return events.events
}
