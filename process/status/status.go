// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"encoding/json"
	"fmt"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

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
	State       string `json:"Juju state" yaml:"Juju state"`
	Info        string `json:"info" yaml:"info"`
	PluginState string `json:"plugin state" yaml:"plugin state"`
}

// Format converts the object returned from the API for our component
// to the object we want to display in the CLI.  In our case, the api object is
// a []process.Info.
func Format(b []byte) interface{} {
	var infos []api.Process
	if err := json.Unmarshal(b, &infos); err != nil {
		return fmt.Errorf("error loading type returned from api: %s", err)
	}

	result := make(map[string]cliDetails, len(infos))
	for _, info := range infos {
		status := api.APIStatus2Status(info.Status)
		result[info.Definition.Name] = cliDetails{
			ID:   info.Details.ID,
			Type: info.Definition.Type,
			Status: cliStatus{
				State:       status.State,
				Info:        status.String(),
				PluginState: info.Details.Status.State,
			},
		}
	}
	return result
}
