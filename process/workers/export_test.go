// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/process"
	"github.com/juju/utils/set"
)

func ExposeChannel(events *EventHandlers) chan []process.Event {
	return events.events
}
