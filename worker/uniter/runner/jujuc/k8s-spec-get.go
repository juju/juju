// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
)

// K8sSpecGetCommand implements the k8s-spec-get command.
type K8sSpecGetCommand struct {
	cmd.CommandBase
	name string
	ctx  Context
}

// NewK8sSpecGetCommand makes a k8s-spec-get command.
func NewK8sSpecGetCommand(ctx Context, name string) (cmd.Command, error) {
	return &K8sSpecGetCommand{ctx: ctx, name: name}, nil
}

func (c *K8sSpecGetCommand) Info() *cmd.Info {
	doc := `
Gets configuration data used to set up k8s resources.
`
	purpose := "get k8s spec information"
	if c.name == "pod-spec-get" {
		purpose += " (deprecated)"
	}
	return jujucmd.Info(&cmd.Info{
		Name:    c.name,
		Purpose: purpose,
		Doc:     doc,
	})
}

func (c *K8sSpecGetCommand) Run(ctx *cmd.Context) error {
	spec, err := c.ctx.GetPodSpec()
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprint(ctx.Stdout, spec)
	return nil
}
