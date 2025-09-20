// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
)

// WatchTaskLogs starts and returns a StringsWatcher that notifies on new log
// messages for a specified action being added. The strings are json encoded
// action messages.
// Returns TaskNotFound if the task does not exist.
func (w *WatchableService) WatchTaskLogs(ctx context.Context, taskID string) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	taskUUID, err := w.st.GetTaskUUIDByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	var (
		logs   []internal.TaskLogMessage
		cursor time.Time
	)

	initialQuery := func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchTaskLogs.initialQuery")
		defer span.End()

		logs, cursor, err = w.st.GetLatestTaskLogsByUUID(ctx, taskUUID, cursor)
		if err != nil {
			return nil, errors.Errorf("initial query for task %q logs: %q", taskID, err)
		}

		return transformLogsToSlice(logs)
	}

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchTaskLogs.mapper")
		defer span.End()

		logs, cursor, err = w.st.GetLatestTaskLogsByUUID(ctx, taskUUID, cursor)
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

// WatchUnitTaskNotifications returns a StringsWatcher that emits task ids
// for tasks targeted at the provided unit.
// NOTE: This watcher will emit events for tasks changing their statuses to
// PENDING or ABORTING only.
func (s *WatchableService) WatchUnitTaskNotifications(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	namespace, initialQuery := s.st.InitialWatchStatementUnitTask()

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchUnitTaskNotifications.mapper")
		defer span.End()

		if len(changes) == 0 {
			return nil, nil
		}

		taskUUIDs := transform.Slice(changes, func(in changestream.ChangeEvent) string {
			return in.Changed()
		})
		taskIDs, err := s.st.FilterTaskUUIDsForUnit(ctx, taskUUIDs, unitUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}

		return taskIDs, nil
	}

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(initialQuery, unitUUID),
		fmt.Sprintf("unit tasks watcher for %q", unitName),
		mapper,
		eventsource.NamespaceFilter(namespace, changestream.Changed),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// WatchMachineTaskNotifications returns a StringsWatcher that emits task
// ids for tasks targeted at the provided machine.
// NOTE: This watcher will emit events for tasks changing their statuses to
// PENDING only.
func (s *WatchableService) WatchMachineTaskNotifications(ctx context.Context, machineName coremachine.Name) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machineUUID, err := s.st.GetMachineUUIDByName(ctx, machineName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	namespace, initialQuery := s.st.InitialWatchStatementMachineTask()

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		ctx, span := trace.Start(ctx, "WatchMachineTaskNotifications.mapper")
		defer span.End()

		if len(changes) == 0 {
			return nil, nil
		}

		taskUUIDs := transform.Slice(changes, func(in changestream.ChangeEvent) string {
			return in.Changed()
		})
		taskIDs, err := s.st.FilterTaskUUIDsForMachine(ctx, taskUUIDs, machineUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}

		return taskIDs, nil
	}

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(initialQuery, machineUUID),
		fmt.Sprintf("machine tasks watcher for %q", machineName),
		mapper,
		eventsource.NamespaceFilter(namespace, changestream.Changed),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
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
