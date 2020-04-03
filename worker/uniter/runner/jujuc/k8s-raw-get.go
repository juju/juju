// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
)

// K8sRawGetCommand implements the k8s-raw-get command.
type K8sRawGetCommand struct {
	cmd.CommandBase
	ctx Context
}

// NewK8sRawGetCommand makes a k8s-raw-get command.
func NewK8sRawGetCommand(ctx Context) (cmd.Command, error) {
	return &K8sRawGetCommand{ctx: ctx}, nil
}

func (c *K8sRawGetCommand) Info() *cmd.Info {
	doc := `
Gets configuration data used to set up k8s resources.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "k8s-raw-get",
		Purpose: "get k8s raw spec information",
		Doc:     doc,
	})
}

func (c *K8sRawGetCommand) Run(ctx *cmd.Context) error {
	spec, err := c.ctx.GetRawK8sSpec()
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprint(ctx.Stdout, spec)
	return nil
}
