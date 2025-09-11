// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// WatchTaskAbortingForReceiver watches for any task for the receiverUUID to
// have a status of aborting. TaskIDs are sent on via the returned strings
// watcher.
func (w *WatchableService) WatchTaskAbortingForReceiver(
	ctx context.Context,
	receiverUUID uuid.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	initialQuery := func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchTaskAbortingForReceiver.initialQuery")
		defer span.End()

		ids, err := w.st.GetIDsForAbortingTaskOfReceiver(ctx, receiverUUID)
		if err != nil {
			return nil, errors.Errorf("%q: %w", receiverUUID, err)
		}

		return ids, err
	}

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchTaskAbortingForReceiver.mapper")
		defer span.End()

		taskUUIDs := transform.Slice(changes, func(in changestream.ChangeEvent) string {
			return in.Changed()
		})

		// The namespace watched, only triggers when a task status is
		// set to ABORTING. Find which tasks are for the receiver
		// provided to the watcher.
		taskIDs, err := w.st.GetTaskIDsByUUIDsFilteredByReceiverUUID(ctx, receiverUUID, taskUUIDs)
		if err != nil {
			return nil, errors.Errorf("task aborted watcher mapper %q: %w", receiverUUID, err)
		}
		return taskIDs, nil
	}

	return w.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("aborting status task watcher for %q", receiverUUID),
		mapper,
		eventsource.NamespaceFilter(w.st.NamespaceForTaskAbortingWatcher(), changestream.Changed),
	)
}

// WatchTaskLogs starts and returns a StringsWatcher that notifies on new log
// messages for a specified action being added. The strings are json encoded
// action messages.
func (w *WatchableService) WatchTaskLogs(ctx context.Context, taskID string) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	taskUUID, err := w.st.GetTaskUUIDByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	var (
		logs []internal.TaskLogMessage
		page int
	)

	initialQuery := func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchTaskLogs initial query")
		defer span.End()

		logs, page, err = w.st.GetPaginatedTaskLogsByUUID(ctx, taskUUID, 0)
		if err != nil {
			return nil, errors.Errorf("initial query for task %q logs: %q", taskID, err)
		}

		return transformLogsToSlice(logs)
	}

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchTaskLogs mapper")
		defer span.End()

		logs, page, err = w.st.GetPaginatedTaskLogsByUUID(ctx, taskUUID, page)
		if err != nil {
			return nil, errors.Capture(err)
		}

		result, err := transformLogsToSlice(logs)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return result, errors.Capture(err)
	}

	return w.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("task log watcher for %q", taskID),
		mapper,
		eventsource.PredicateFilter(w.st.NamespaceForTaskLogWatcher(), changestream.Changed, eventsource.EqualsPredicate(taskUUID)),
	)
}

func transformLogsToSlice(msgs []internal.TaskLogMessage) ([]string, error) {
	return transform.SliceOrErr(msgs, func(in internal.TaskLogMessage) (string, error) {
		str, err := in.TransformToCore().Encode()
		if err != nil {
			return "", errors.Errorf("encoding log for watcher: %w", err)
		}
		return str, nil
	})
}
