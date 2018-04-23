// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

//find all the agents available in the machine
func FindAgents(machineAgent *string, unitAgents *[]string, dataDir string) error {
	agentDir := filepath.Join(dataDir, "agents")
	dir, err := os.Open(agentDir)
	if err != nil {
		return errors.Annotate(err, "opening agents dir")
	}
	defer dir.Close()

	entries, err := dir.Readdir(-1)
	if err != nil {
		return errors.Annotate(err, "reading agents dir")
	}
	for _, info := range entries {
		name := info.Name()
		tag, err := names.ParseTag(name)
		if err != nil {
			continue
		}
		switch tag.Kind() {
			case names.MachineTagKind:
				*machineAgent = name
			case names.UnitTagKind:
				*unitAgents = append(*unitAgents, name)
			default:
				errors.Errorf("%s is not of type Machine nor Unit, ignoring", name)
		}
	}
	return nil
}
