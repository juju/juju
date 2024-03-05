// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import "github.com/juju/errors"

// ListMachineTypes returns a list of MachineType available for the
// given zone.
func (gce *Connection) ListMachineTypes(zone string) ([]MachineType, error) {
	machines, err := gce.service.ListMachineTypes(gce.projectID, zone)
	if err != nil {
		return nil, errors.Trace(err)
	}
	res := make([]MachineType, len(machines.Items))
	for i, machine := range machines.Items {
		deprecated := false
		if machine.Deprecated != nil {
			deprecated = machine.Deprecated.State != ""
		}
		res[i] = MachineType{
			CreationTimestamp:            machine.CreationTimestamp,
			Deprecated:                   deprecated,
			Description:                  machine.Description,
			GuestCpus:                    machine.GuestCpus,
			Id:                           machine.Id,
			ImageSpaceGb:                 machine.ImageSpaceGb,
			Kind:                         machine.Kind,
			MaximumPersistentDisks:       machine.MaximumPersistentDisks,
			MaximumPersistentDisksSizeGb: machine.MaximumPersistentDisksSizeGb,
			MemoryMb:                     machine.MemoryMb,
			Name:                         machine.Name,
		}
	}
	return res, nil
}
