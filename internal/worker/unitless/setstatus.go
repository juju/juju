// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"github.com/canonical/starlark/starlark"

	"github.com/juju/juju/internal/errors"
)

// IntentSetStatus indicates an application status update.
const IntentSetStatus IntentType = "set-status"

const setStatusSafety = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

var setStatusBuiltin = starlark.NewBuiltinWithSafety("set_status", setStatusSafety, setStatus)

func setStatus(
	thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var status, message string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "status", &status, "message?", &message); err != nil {
		return nil, err
	}

	collector, err := intentCollectorForThread(thread)
	if err != nil {
		return nil, errors.Errorf("getting IntentCollector from thread: %w", err)
	}
	intentArgs := map[string]any{
		"status":  status,
		"message": message,
	}
	if err := thread.AddAllocs(starlark.EstimateSize(intentArgs)); err != nil {
		return nil, errors.Errorf("adding allocations for set-status arguments: %w", err)
	}
	err = collector.append(thread, Intent{
		Type: IntentSetStatus,
		Args: intentArgs,
	})
	if err != nil {
		return nil, errors.Errorf("appending set-status intent: %w", err)
	}
	return starlark.None, nil
}
