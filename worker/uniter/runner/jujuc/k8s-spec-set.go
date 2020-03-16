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

// K8sSpecSetCommand implements the k8s-spec-set command.
type K8sSpecSetCommand struct {
	cmd.CommandBase
	name string
	ctx  Context

	specFile     cmd.FileVar
	k8sResources cmd.FileVar
}

// NewK8sSpecSetCommand makes a k8s-spec-set command.
func NewK8sSpecSetCommand(ctx Context, name string) (cmd.Command, error) {
	return &K8sSpecSetCommand{ctx: ctx, name: name}, nil
}

func (c *K8sSpecSetCommand) Info() *cmd.Info {
	doc := `
Sets configuration data to use for k8s resources.
The spec applies to all units for the application.
`
	purpose := "set k8s spec information"
	if c.name == "pod-spec-set" {
		purpose += " (deprecated)"
	}
	return jujucmd.Info(&cmd.Info{
		Name:    c.name,
		Args:    "--file <core spec file> [--k8s-resources <k8s spec file>]",
		Purpose: purpose,
		Doc:     doc,
	})
}

func (c *K8sSpecSetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.specFile.SetStdin()
	c.specFile.Path = "-"
	f.Var(&c.specFile, "file", "file containing pod spec")
	f.Var(&c.k8sResources, "k8s-resources", "file containing k8s specific resources not yet modelled by Juju")
}

func (c *K8sSpecSetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *K8sSpecSetCommand) Run(ctx *cmd.Context) error {
	specData, err := c.mergePodSpec(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return c.ctx.SetPodSpec(specData)
}

func (c *K8sSpecSetCommand) mergePodSpec(ctx *cmd.Context) (string, error) {
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

func (c *K8sSpecSetCommand) handleSpecFile(ctx *cmd.Context) (string, error) {
	specData, err := c.specFile.Read(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(specData) == 0 {
		return "", errors.New("no k8s spec specified: pipe k8s spec to command, or specify a file with --file")
	}
	return string(specData), nil
}

func (c *K8sSpecSetCommand) handleK8sResources(ctx *cmd.Context) (string, error) {
	if c.k8sResources.Path == "" {
		return "", nil
	}
	k8sResourcesData, err := c.k8sResources.Read(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(k8sResourcesData), nil
}
