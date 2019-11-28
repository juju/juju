// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
)

// PodSpecGetCommand implements the pod-spec-get command.
type PodSpecGetCommand struct {
	cmd.CommandBase
	ctx Context
}

// NewPodSpecGetCommand makes a pod-spec-get command.
func NewPodSpecGetCommand(ctx Context) (cmd.Command, error) {
	return &PodSpecGetCommand{ctx: ctx}, nil
}

func (c *PodSpecGetCommand) Info() *cmd.Info {
	doc := `
Gets configuration data used for a pod.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "pod-spec-get",
		Purpose: "get pod spec information",
		Doc:     doc,
	})
}

func (c *PodSpecGetCommand) Run(ctx *cmd.Context) error {
	spec, err := c.ctx.GetPodSpec()
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprint(ctx.Stdout, spec)
	return nil
}
