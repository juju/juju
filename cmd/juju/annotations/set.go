// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/annotations"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// SetAnnotationsAPI is the annotations client API.
type SetAnnotationsAPI interface {
	Set(annotations map[string]map[string]string) ([]params.ErrorResult, error)
	Close() error
}

const (
	setAnnotationsDoc = `
Set annotations for an entity with a list of key values.
`
	setAnnotationsExamples = `
	juju set-annotations model-<modelUUID> owner=alice
	juju set-annotations applicationoffer-<offerUUID> stage=staging owner=eve
`
)

type setAnnotationsCommand struct {
	modelcmd.ModelCommandBase

	resourceTag        names.Tag
	annotations        map[string]string
	annotationsAPIFunc func() (SetAnnotationsAPI, error)
}

// NewSetAnnotationsCommand returns a command to set annotations for juju resources.
func NewSetAnnotationsCommand() cmd.Command {
	c := &setAnnotationsCommand{}
	c.annotationsAPIFunc = c.annotationsAPI

	return modelcmd.Wrap(c)
}

func (c *setAnnotationsCommand) annotationsAPI() (SetAnnotationsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return annotations.NewClient(root), nil
}

// Info implements cmd.Command.
func (c *setAnnotationsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "set-annotations",
		Args:     "<resource tag> [key=value...]",
		Purpose:  "Set annotations.",
		Doc:      setAnnotationsDoc,
		Examples: setAnnotationsExamples,
	})
}

// Init implements cmd.Command.
func (c *setAnnotationsCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("resource tag needs to be supplied as the first argument")
	}

	tag, err := names.ParseTag(args[0])
	if err != nil {
		return err
	}
	c.resourceTag = tag
	args = args[1:]

	c.annotations = make(map[string]string)

	for _, arg := range args {
		tokens := strings.Split(arg, "=")
		if len(tokens) != 2 {
			return errors.Errorf("invalid key value pair: %s", arg)
		}
		c.annotations[tokens[0]] = tokens[1]
	}
	return nil
}

// Run implements cmd.Command.
func (c *setAnnotationsCommand) Run(ctx *cmd.Context) error {
	api, err := c.annotationsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	results, err := api.Set(map[string]map[string]string{
		c.resourceTag.String(): c.annotations,
	})
	if err != nil {
		return err
	}
	if len(results) > 0 && results[0].Error != nil {
		return results[0].Error
	}
	return nil
}
