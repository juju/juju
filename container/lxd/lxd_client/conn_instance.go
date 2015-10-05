// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"strings"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared"
)

// TODO(ericsnow) We probably need to address some of the things that
// get handled in container/lxc/clonetemplate.go.

func (client *Client) addInstance(spec InstanceSpec) error {
	remote := ""
	//remote := client.remote
	//remote := spec.Remote
	imageAlias := "ubuntu" // TODO(ericsnow) Do not hard-code.
	//image := spec.Image
	var profiles *[]string
	if len(spec.Profiles) > 0 {
		profiles = &spec.Profiles
	}

	resp, err := client.raw.Init(spec.Name, remote, imageAlias, profiles, spec.Ephemeral)
	if err != nil {
		return errors.Trace(err)
	}

	// Init is an async operation, since the tar -xvf (or whatever) might
	// take a while; the result is an LXD operation id, which we can just
	// wait on until it is finished.
	if err := client.raw.WaitForSuccess(resp.Operation); err != nil {
		// TODO(ericsnow) Handle different failures (from the async
		// operation) differently?
		return errors.Trace(err)
	}

	return nil
}

func (client *Client) startInstance(spec InstanceSpec) error {
	timeout := -1
	force := false
	resp, err := client.raw.Action(spec.Name, shared.Start, timeout, force)
	if err != nil {
		return errors.Trace(err)
	}

	if err := client.raw.WaitForSuccess(resp.Operation); err != nil {
		// TODO(ericsnow) Handle different failures (from the async
		// operation) differently?
		return errors.Trace(err)
	}

	return nil
}

// AddInstance creates a new instance based on the spec's data and
// returns it. The instance will be created using the client.
func (client *Client) AddInstance(spec InstanceSpec) (*Instance, error) {
	if err := client.addInstance(spec); err != nil {
		return nil, errors.Trace(err)
	}

	if err := client.startInstance(spec); err != nil {
		if err := client.removeInstance(spec.Name); err != nil {
			logger.Errorf("could not remove container %q after starting it failed", spec.Name)
		}
		return nil, errors.Trace(err)
	}

	inst, err := client.Instance(spec.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	inst.spec = &spec

	return inst, nil
}

// Instance gets the up-to-date info about the given instance
// and returns it.
func (client *Client) Instance(name string) (*Instance, error) {
	showLog := false
	info, err := client.raw.ContainerStatus(name, showLog)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := newInstance(info, nil)
	return inst, nil
}

// Instances sends a request to the API for a list of all instances
// (in the Client's namespace) for which the name starts with the
// provided prefix. The result is also limited to those instances with
// one of the specified statuses (if any).
func (client *Client) Instances(prefix string, statuses ...string) ([]Instance, error) {
	infos, err := client.raw.ListContainers()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var insts []Instance
	for _, info := range infos {
		name := info.State.Name
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		if len(statuses) > 0 && !checkStatus(info, statuses) {
			continue
		}

		inst := newInstance(&info.State, nil)
		insts = append(insts, *inst)
	}
	return insts, nil
}

func checkStatus(info shared.ContainerInfo, statuses []string) bool {
	for _, status := range statuses {
		statusCode := allStatuses[status]
		if info.State.Status.StateCode == statusCode {
			return true
		}
	}
	return false
}

// removeInstance sends a request to the API to remove the instance
// with the provided ID. The call blocks until the instance is removed
// (or the request fails).
func (client *Client) removeInstance(name string) error {
	info, err := client.raw.ContainerStatus(name)
	if err != nil {
		return errors.Trace(err)
	}

	//if info.Status.StatusCode != 0 && info.Status.StatusCode != shared.Stopped {
	if info.Status.StatusCode != shared.Stopped {
		timeout := -1
		force := true
		resp, err := client.raw.Action(name, shared.Stop, timeout, force)
		if err != nil {
			return errors.Trace(err)
		}

		if err := client.raw.WaitForSuccess(resp.Operation); err != nil {
			// TODO(ericsnow) Handle different failures (from the async
			// operation) differently?
			return errors.Trace(err)
		}
	}

	resp, err := client.raw.Delete(name)
	if err != nil {
		return errors.Trace(err)
	}

	if err := client.raw.WaitForSuccess(resp.Operation); err != nil {
		// TODO(ericsnow) Handle different failures (from the async
		// operation) differently?
		return errors.Trace(err)
	}

	return nil
}

// RemoveInstances sends a request to the API to terminate all
// instances (in the Client's namespace) that match one of the
// provided IDs. If a prefix is provided, only IDs that start with the
// prefix will be considered. The call blocks until all the instances
// are removed or the request fails.
func (client *Client) RemoveInstances(prefix string, names ...string) error {
	if len(names) == 0 {
		return nil
	}

	instances, err := client.Instances(prefix)
	if err != nil {
		return errors.Annotatef(err, "while removing instances %v", names)
	}

	var failed []string
	for _, name := range names {
		if !checkInstanceName(name, instances) {
			// We ignore unknown instance names.
			continue
		}

		if err := client.removeInstance(name); err != nil {
			failed = append(failed, name)
			logger.Errorf("while removing instance %q: %v", name, err)
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("some instance removals failed: %v", failed)
	}
	return nil
}

func checkInstanceName(name string, instances []Instance) bool {
	for _, inst := range instances {
		if inst.Name == name {
			return true
		}
	}
	return false
}
