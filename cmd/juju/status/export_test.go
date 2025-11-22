// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
)

// NewStatusHistoryCommandForTest creates a new instance of a statusHistoryCommand
// for testing, using the provided HistoryAPI. It supports mocking several
// controllers
func NewStatusHistoryCommandForTest(apis ...HistoryAPI) cmd.Command {
	return &statusHistoryCommand{getStatusHistoryCollectors: func() ([]historyCollector, error) {
		var collectors []historyCollector
		for _, api := range apis {
			collectors = append(collectors, toCollector(api.StatusHistory))
		}
		return collectors, nil
	}}
}

// toCollector creates a historyCollector by wrapping a function that retrieves
// status history into the collectorResult format.
func toCollector(history func(ctx context.Context, kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error)) historyCollector {
	return func(ctx context.Context, kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) collectorResult {
		v, err := history(ctx, kind, tag, filter)
		return collectorResult{history: v, err: err}
	}
}

// NewStatusCommandForTest creates a new instance of a statusCommand for testing,
// using the provided statusAPI and clock.
func NewStatusCommandForTest(store jujuclient.ClientStore, statusapi statusAPI, clock Clock) cmd.Command {
	cmd := &statusCommand{statusAPI: statusapi, clock: clock}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
