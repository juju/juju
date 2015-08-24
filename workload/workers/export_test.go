// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/worker"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

func ExposeEventHandlers(eh *EventHandlers) (*Events, []func([]workload.Event, context.APIClient, Runner) error, context.APIClient, worker.Runner) {
	return eh.events, eh.handlers, eh.apiClient, eh.runner
}

func ExposeEvents(e *Events) chan []workload.Event {
	return e.events
}
