// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"encoding/json"
	"fmt"

	"github.com/juju/juju/process/api"
)

type cliDetails struct {
	ID     string    `json:"id" yaml:"id"`
	Type   string    `json:"type" yaml:"type"`
	Status cliStatus `json:"status" yaml:"status"`
}

type cliStatus struct {
	State string `json:"state" yaml:"state"`
}

// convertAPItoCLI converts the object returned from the API for our component
// to the object we want to display in the CLI.  In our case, the api object is
// a []process.Info.
func ConvertAPItoCLI(apiObj interface{}) (cliObj interface{}) {
	if apiObj == nil {
		return nil
	}
	var infos []api.Process

	// ok, this is ugly, but because our type was unmarshaled into a
	// map[string]interface{}, the easiest way to convert it into the type we
	// want is just to marshal it back out and then unmarshal it into the
	// correct type.
	b, err := json.Marshal(apiObj)
	if err != nil {
		return fmt.Sprintf("error reading type returned from api: %s", err)
	}

	if err := json.Unmarshal(b, &infos); err != nil {
		return fmt.Sprintf("error loading type returned from api: %s", err)
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
