// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"github.com/canonical/starlark/starlark"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

const setStatusSafety = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

var setStatusBuiltin = starlark.NewBuiltinWithSafety("set_status", setStatusSafety, setStatus)

// StatusCollector retains the last status set during an event.
type StatusCollector struct {
	Status  string
	Message string
}

func setStatus(
	thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var status, message string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "status", &status, "message?", &message); err != nil {
		return nil, err
	}

	if status == "" {
		return nil, errors.New("status must not be empty").Add(coreerrors.NotValid)
	}

	statusCollector, err := intentCollectorForThread[*StatusCollector](thread)
	if err != nil {
		return nil, errors.Errorf("getting status collector from thread: %w", err)
	}
	statusCollector.Status = status
	statusCollector.Message = message
	return starlark.None, nil
}
