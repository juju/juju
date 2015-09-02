// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"fmt"
	"strings"
)

// PortAssignment describes a port mapping between the host
// and the container.
type PortAssignment struct {
	// External is the port on the host.
	External int
	// Internal is the port on the container.
	Internal int
	// Protocol is the network protocol for the mapping (e.g. tcp, udp).
	Protocol string
}

// String returns a docker-friendly string representation of the mapping.
func (pa PortAssignment) String() string {
	return fmt.Sprintf("%d:%d/%s", pa.External, pa.Internal, pa.Protocol)
}

// MountAssignment describes a volume mount mapping between the host
// and the container.
type MountAssignment struct {
	// External is the volume mount point on the host.
	External string
	// Internal is the volume mount point on the container.
	Internal string
	// Mode is the docker-recognized access mode (e.g. rw, ro).
	Mode string
}

// String returns a docker-friendly string representation of the mapping.
func (ma MountAssignment) String() string {
	return fmt.Sprintf("%s:%s:%s", ma.External, ma.Internal, ma.Mode)
}

// RunArgs contains the data passed to the Run function.
type RunArgs struct {
	// Name is the unique name to assign to the container (optional).
	Name string
	// Image is the container image to use.
	Image string
	// Command is the command to run in the container (optional).
	Command string
	// EnvVars holds the environment variables to use in the container,
	// if any.
	EnvVars map[string]string
	// Ports holds the ports info to map into the container from the
	// host, if any.
	Ports []PortAssignment
	// Mounts holds the volumes info to map into the container from the
	// host, if any.
	Mounts []MountAssignment
}

// CommandlineArgs converts the RunArgs into a list of strings that may
// be passed to exec.Command as the command args.
func (ra RunArgs) CommandlineArgs() []string {
	args := []string{
		"--detach",
	}

	if ra.Name != "" {
		args = append(args, "--name", ra.Name)
	}

	for k, v := range ra.EnvVars {
		args = append(args, "-e", k+"="+v)
	}

	for _, p := range ra.Ports {
		args = append(args, "-p", p.String())
	}

	for _, m := range ra.Mounts {
		args = append(args, "-v", m.String())
	}

	// Image and Command must come after all options.
	args = append(args, ra.Image)

	if ra.Command != "" {
		args = append(args, strings.Fields(ra.Command)...)
	}

	return args
}
