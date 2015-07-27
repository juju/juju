// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"encoding/json"
	"fmt"

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

type cliDetails struct {
	ID     string    `json:"id" yaml:"id"`
	Type   string    `json:"type" yaml:"type"`
	Status cliStatus `json:"status" yaml:"status"`
}

type cliStatus struct {
	State string `json:"state" yaml:"state"`
}

// Format converts the object returned from the API for our component
// to the object we want to display in the CLI.  In our case, the api object is
// a []process.Info.
func Format(apiObj interface{}) interface{} {
	if apiObj == nil {
		return nil
	}
	var infos []api.Process

	// ok, this is ugly, but because our type was unmarshaled into a
	// []map[string]interface{}, the easiest way to convert it into the type we
	// want is just to marshal it back out and then unmarshal it into the
	// correct type.
	b, err := json.Marshal(apiObj)
	if err != nil {
		return fmt.Errorf("error reading type returned from api: %s", err)
	}

	if err := json.Unmarshal(b, &infos); err != nil {
		return fmt.Errorf("error loading type returned from api: %s", err)
	}

	result := map[string]cliDetails{}
	for _, info := range infos {
		result[info.Definition.Name] = cliDetails{
			ID:   info.Details.ID,
			Type: info.Definition.Type,
			Status: cliStatus{
				State: info.Details.Status.Label,
			},
		}
	}
	return result
}
