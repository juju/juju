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
	run     func(docker.RunArgs, func(string, ...string) ([]byte, error)) (id string, err error)
	inspect func(string, func(string, ...string) ([]byte, error)) (*docker.Info, error)
	stop    func(string, func(string, ...string) ([]byte, error)) error
	remove  func(string, func(string, ...string) ([]byte, error)) error

	exec func(string, ...string) ([]byte, error)
}

// NewDockerPlugin returns a DockerPlugin.
func NewDockerPlugin(exec func(string, ...string) ([]byte, error)) *DockerPlugin {
	p := &DockerPlugin{
		run:     docker.Run,
		inspect: docker.Inspect,
		stop:    docker.Stop,
		remove:  docker.Remove,

		exec: exec,
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
	id, err := p.run(args, p.exec)
	if err != nil {
		return details, errors.Trace(err)
	}

	info, err := p.inspect(id, p.exec)
	if err != nil {
		return details, errors.Annotatef(err, "can't get status for container %q", id)
	}

	details.ID = strings.TrimPrefix(info.Name, "/")
	details.Status.State = info.Process.State.String()
	return details, nil
}

// Status returns the ProcStatus for the docker container with the given id.
func (p DockerPlugin) Status(id string) (workload.PluginStatus, error) {
	dockerLogger.Debugf("getting status for %q", id)

	var status workload.PluginStatus
	info, err := p.inspect(id, p.exec)
	if err != nil {
		return status, errors.Trace(err)
	}
	status.State = info.Process.State.String()
	return status, nil
}

// Destroy stops and removes the docker container with the given id.
func (p DockerPlugin) Destroy(id string) error {
	dockerLogger.Debugf("destroying %q", id)

	if err := p.stop(id, p.exec); err != nil {
		return errors.Trace(err)
	}
	if err := p.remove(id, p.exec); err != nil {
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
