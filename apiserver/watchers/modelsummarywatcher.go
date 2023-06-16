// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// newModelSummaryWatcher exists solely to be registered with regRaw.
// Standard registration doesn't handle watcher types (it checks for
// and empty ID in the context).
func NewModelSummaryWatcher(context facade.Context) (facade.Facade, error) {
	return newModelSummaryWatcher(context)
}

// NewModelSummaryWatcher returns a new API server endpoint for interacting with
// a watcher created by the WatchModelSummaries and WatchAllModelSummaries API
// calls.
func newModelSummaryWatcher(context facade.Context) (*srvModelSummaryWatcher, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
	)

	if !auth.AuthClient() {
		// Note that we don't need to check specific permissions
		// here, as the AllWatcher can only do anything if the
		// watcher resource has already been created, so we can
		// rely on the permission check there to ensure that
		// this facade can't do anything it shouldn't be allowed
		// to.
		//
		// This is useful because the AllWatcher is reused for
		// both the WatchAll (requires model access rights) and
		// the WatchAllModels (requring controller superuser
		// rights) API calls.
		return nil, apiservererrors.ErrPerm
	}

	watcher, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelSummaryWatcher, ok := watcher.(corewatcher.ModelSummaryWatcher)
	if !ok {
		return nil, errors.Annotatef(apiservererrors.ErrUnknownWatcher, "watcher id: %s", id)
	}
	return &srvModelSummaryWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       modelSummaryWatcher,
	}, nil
}

// srvModelSummaryWatcher defines the API methods on a ModelSummaryWatcher.
type srvModelSummaryWatcher struct {
	watcherCommon
	watcher corewatcher.ModelSummaryWatcher
}

// Next will return the current state of everything on the first call
// and subsequent calls will return just those model summaries that have
// changed.
func (w *srvModelSummaryWatcher) Next() (params.SummaryWatcherNextResults, error) {
	if summaries, ok := <-w.watcher.Changes(); ok {
		return params.SummaryWatcherNextResults{
			Models: w.translate(summaries),
		}, nil
	}
	return params.SummaryWatcherNextResults{}, apiservererrors.ErrStoppedWatcher
}

func (w *srvModelSummaryWatcher) translate(summaries []corewatcher.ModelSummary) []params.ModelAbstract {
	response := make([]params.ModelAbstract, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Removed {
			response = append(response, params.ModelAbstract{
				UUID:    summary.UUID,
				Removed: true,
			})
			continue
		}

		result := params.ModelAbstract{
			UUID:       summary.UUID,
			Controller: summary.Controller,
			Name:       summary.Name,
			Admins:     summary.Admins,
			Cloud:      summary.Cloud,
			Region:     summary.Region,
			Credential: summary.Credential,
			Size: params.ModelSummarySize{
				Machines:     summary.MachineCount,
				Containers:   summary.ContainerCount,
				Applications: summary.ApplicationCount,
				Units:        summary.UnitCount,
				Relations:    summary.RelationCount,
			},
			Status:      summary.Status,
			Messages:    w.translateMessages(summary.Messages),
			Annotations: summary.Annotations,
		}
		response = append(response, result)
	}
	return response
}

func (w *srvModelSummaryWatcher) translateMessages(messages []corewatcher.ModelSummaryMessage) []params.ModelSummaryMessage {
	if messages == nil {
		return nil
	}
	result := make([]params.ModelSummaryMessage, len(messages))
	for i, m := range messages {
		result[i] = params.ModelSummaryMessage{
			Agent:   m.Agent,
			Message: m.Message,
		}
	}
	return result
}
