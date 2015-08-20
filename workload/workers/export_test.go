// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/workload"
	"github.com/juju/utils/set"
)

func ExposeChannel(events *EventHandlers) chan []workload.Event {
	return events.events
}

func ExposeRunner(runner Runner) (Runner, set.Strings) {
	tracking, ok := runner.(*trackingRunner)
	if !ok {
		return runner, nil
	}
	return tracking.Runner, tracking.running
}
