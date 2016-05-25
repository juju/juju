// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
)

// Hostname returns a name suitable to be used for a machine hostname.
// This function returns an error if either the model or machine tags are invalid.
func Hostname(model names.ModelTag, machine names.MachineTag) (string, error) {
	uuid := model.Id()
	// TODO: would be nice if the tags exported a method Valid().
	if !names.IsValidModel(uuid) {
		return "", errors.Errorf("model ID %q is not a valid model", uuid)
	}
	// The suffix is the last six hex digits of the model uuid.
	suffix := uuid[len(uuid)-6:]

	machineID := machine.Id()
	if !names.IsValidMachine(machineID) {
		return "", errors.Errorf("machine ID %q is not a valid machine", machineID)
	}
	machineID = strings.Replace(machineID, "/", "-", -1)

	return "juju-" + suffix + "-" + machineID, nil
}
