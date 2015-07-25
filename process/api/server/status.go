// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

const StatusType = "workload-processes"

// UnitStatus returns a status object to be returned by juju status.
func UnitStatus(procs []process.Info) (interface{}, error) {
	if len(procs) == 0 {
		return nil, nil
	}

	results := make([]api.Process, len(procs))
	for i, p := range procs {
		results[i] = api.Proc2api(p)
	}
	return results, nil
}
