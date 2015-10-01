// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared"
)

// TODO(ericsnow) Return a new inst.
func (client *Client) addInstance(spec InstanceSpec) error {
	return errors.NotImplementedf("")
}

// AddInstance creates a new instance based on the spec's data and
// returns it. The instance will be created using the client.
func (client *Client) AddInstance(spec InstanceSpec) (*Instance, error) {
	if err := client.addInstance(spec); err != nil {
		return nil, errors.Trace(err)
	}

	return nil, errors.NotImplementedf("")
	// TODO(ericsnow) Pull the instance info via the API.
	var info *shared.ContainerState
	inst := newInstance(info, &spec)
	return inst, nil
}

// Instance gets the up-to-date info about the given instance
// and returns it.
func (client *Client) Instance(id string) (*Instance, error) {
	return nil, errors.NotImplementedf("")
	var info *shared.ContainerState
	result := newInstance(info, nil)
	return result, nil
}

// Instances sends a request to the API for a list of all instances
// (in the Client's namespace) for which the name starts with the
// provided prefix. The result is also limited to those instances with
// one of the specified statuses (if any).
func (client *Client) Instances(prefix string, statuses ...string) ([]Instance, error) {
	return nil, errors.NotImplementedf("")

	infos, err := client.raw.ListContainers()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// convert statuses using allStatuses and then filter...

	var insts []Instance
	for _, info := range infos {
		inst := newInstance(&info.State, nil)
		insts = append(insts, *inst)
	}
	return insts, nil
}

// removeInstance sends a request to the API to remove the instance
// with the provided ID. The call blocks until the instance is removed
// (or the request fails).
func (client *Client) removeInstance(id string) error {
	return errors.NotImplementedf("")
}

// RemoveInstances sends a request to the API to terminate all
// instances (in the Client's namespace) that match one of the
// provided IDs. If a prefix is provided, only IDs that start with the
// prefix will be considered. The call blocks until all the instances
// are removed or the request fails.
func (client *Client) RemoveInstances(prefix string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	// TODO(ericsnow) Just pull the IDs.
	instances, err := client.Instances(prefix)
	if err != nil {
		return errors.Annotatef(err, "while removing instances %v", ids)
	}

	// TODO(ericsnow) Remove instances in parallel?
	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.ID == instID {
				if err := client.removeInstance(instID); err != nil {
					failed = append(failed, instID)
					logger.Errorf("while removing instance %q: %v", instID, err)
				}
				break
			}
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("some instance removals failed: %v", failed)
	}
	return nil
}
