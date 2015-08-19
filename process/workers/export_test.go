// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/worker"
)

func ExposeEventHandlers(eh *EventHandlers) (chan []process.Event, []func([]process.Event, context.APIClient, Runner) error, context.APIClient, worker.Runner) {
	return eh.events, eh.handlers, eh.apiClient, eh.runner
}
