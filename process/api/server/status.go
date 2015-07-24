// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/names"

	"github.com/juju/juju/process/api"
	"github.com/juju/juju/state"
)

const StatusType = "workload-processes"

type ProcessHolder interface {
	UnitProcesses(names.UnitTag) (state.UnitProcesses, error)
}

// UnitStatus returns a status object to be returned by juju status.
func UnitStatus(p ProcessHolder, unitTag names.UnitTag) (interface{}, error) {
	unitProcesses, err := p.UnitProcesses(unitTag)
	if err != nil {
		return nil, err
	}

	procs, err := unitProcesses.List()
	if err != nil {
		return nil, err
	}

	results := make([]api.Process, len(procs))
	for i, p := range procs {
		results[i] = api.Proc2api(p)
	}
	return results, nil
}
