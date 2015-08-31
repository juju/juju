// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju-process-docker/docker"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

var dockerLogger = loggo.GetLogger("juju.workload.plugin.docker")

// DockerPlugin is an implementation of workload.Plugin for docker.
type DockerPlugin struct {
	// Client is the docker client to use for the plugin.
	Client docker.Client
}

// NewDockerPlugin returns a DockerPlugin.
func NewDockerPlugin() *DockerPlugin {
	p := &DockerPlugin{
		Client: docker.NewCLIClient(),
	}
	return p
}

// Launch runs a new docker container with the given workload data.
func (p DockerPlugin) Launch(definition charm.Workload) (workload.Details, error) {
	dockerLogger.Debugf("launching %q", definition.Name)

	var details workload.Details
	if err := definition.Validate(); err != nil {
		return details, errors.Annotatef(err, "invalid proc-info")
	}

	args := runArgs(definition)
	id, err := p.Client.Run(args)
	if err != nil {
		return details, errors.Trace(err)
	}

	info, err := p.Client.Inspect(id)
	if err != nil {
		return details, errors.Annotatef(err, "can't get status for container %q", id)
	}

	details.ID = strings.TrimPrefix(info.Name, "/")
	details.Status.State = info.StateValue()
	return details, nil
}

// Status returns the ProcStatus for the docker container with the given id.
func (p DockerPlugin) Status(id string) (workload.PluginStatus, error) {
	dockerLogger.Debugf("getting status for %q", id)

	var status workload.PluginStatus
	info, err := p.Client.Inspect(id)
	if err != nil {
		return status, errors.Trace(err)
	}
	status.State = info.StateValue()
	return status, nil
}

// Destroy stops and removes the docker container with the given id.
func (p DockerPlugin) Destroy(id string) error {
	dockerLogger.Debugf("destroying %q", id)

	if err := p.Client.Stop(id); err != nil {
		return errors.Trace(err)
	}
	if err := p.Client.Remove(id); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// runArgs converts the Workload struct into arguments for the
// docker run command.
func runArgs(definition charm.Workload) docker.RunArgs {
	args := docker.RunArgs{
		Name:  definition.Name,
		Image: definition.Image,
	}

	if definition.EnvVars != nil {
		args.EnvVars = make(map[string]string, len(definition.EnvVars))
		for name, value := range definition.EnvVars {
			args.EnvVars[name] = value
		}
	}

	for _, port := range definition.Ports {
		// TODO(natefinch): update this when we use portranges
		args.Ports = append(args.Ports, docker.PortAssignment{
			External: port.External,
			Internal: port.Internal,
			Protocol: "tcp",
		})
	}

	for _, vol := range definition.Volumes {
		// TODO(natefinch): update this when we use portranges
		args.Mounts = append(args.Mounts, docker.MountAssignment{
			External: vol.ExternalMount,
			Internal: vol.InternalMount,
			Mode:     vol.Mode,
		})
	}

	// TODO(natefinch): update this when we make command a list of strings
	if definition.Command != "" {
		args.Command = definition.Command
	}

	return args
}
