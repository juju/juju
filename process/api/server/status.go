// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"fmt"

	"github.com/juju/names"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/process"
)

const StatusType = "workload-processes"

// GetProcessStateFn is a function that returns process status
// information.
type GetProcessStateFn func(names.UnitTag) ([]process.Info, error)

// BuildStatus retrieves a list of process info
func BuildStatus(getProcessList GetProcessStateFn, unitTag names.UnitTag) (map[string]string, error) {
	processList, err := getProcessList(unitTag)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(processList))
	for _, processInfo := range processList {
		result[processInfo.ID()] = processStatusToYaml(processInfo)
	}

	return result, nil
}

func processStatusToYaml(processStatus process.Info) string {
	yaml, err := goyaml.Marshal(processStatus.Details.Status)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	return string(yaml)
}
