// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"bytes"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// PodSpecSetCommand implements the pod-spec-set command.
type PodSpecSetCommand struct {
	cmd.CommandBase
	ctx Context

	specFile     cmd.FileVar
	k8sResources cmd.FileVar
}

// NewPodSpecSetCommand makes a pod-spec-set command.
func NewPodSpecSetCommand(ctx Context) (cmd.Command, error) {
	return &PodSpecSetCommand{ctx: ctx}, nil
}

func (c *PodSpecSetCommand) Info() *cmd.Info {
	doc := `
Sets configuration data to use for a pod.
The spec applies to all units for the application.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "pod-spec-set",
		Args:    "--file <pod spec file> [--k8s-resources <k8s pod spec file>]",
		Purpose: "set pod spec information",
		Doc:     doc,
	})
}

func (c *PodSpecSetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.specFile.SetStdin()
	c.specFile.Path = "-"
	f.Var(&c.specFile, "file", "file containing pod spec")
	f.Var(&c.k8sResources, "k8s-resources", "file containing k8s pod spec")
}

func (c *PodSpecSetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *PodSpecSetCommand) Run(ctx *cmd.Context) error {
	specData, err := c.mergePodSpec(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return c.ctx.SetPodSpec(specData)
}

func (c *PodSpecSetCommand) mergePodSpec(ctx *cmd.Context) (string, error) {
	caasSpecData, err := c.handleSpecFile(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	k8sSpecData, err := c.handleK8sResources(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	return mergeFileContent(caasSpecData, k8sSpecData), nil
}

func mergeFileContent(contents ...string) string {
	var buffer bytes.Buffer
	for _, v := range contents {
		if v == "" {
			continue
		}
		buffer.WriteString(v)
	}
	return buffer.String()
}

func (c *PodSpecSetCommand) handleSpecFile(ctx *cmd.Context) (string, error) {
	specData, err := c.specFile.Read(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(specData) == 0 {
		return "", errors.New("no pod spec specified: pipe pod spec to command, or specify a file with --file")
	}
	return string(specData), nil
}

func (c *PodSpecSetCommand) handleK8sResources(ctx *cmd.Context) (string, error) {
	if c.k8sResources.Path == "" {
		return "", nil
	}
	k8sResourcesData, err := c.k8sResources.Read(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(k8sResourcesData), nil
}
