// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// StorageAddCommand implements the status-set command.
type StorageAddCommand struct {
	cmd.CommandBase
	ctx Context
	all map[string]params.StorageConstraints
}

// NewStorageAddCommand makes a jujuc storage-add command.
func NewStorageAddCommand(ctx Context) (cmd.Command, error) {
	return &StorageAddCommand{ctx: ctx}, nil
}

func (s *StorageAddCommand) Info() *cmd.Info {
	var doc = `
Storage add adds storage instances to unit using provided storage directives.
A storage directive consists of a storage name as per charm specification
and optional storage COUNT.

COUNT is a positive integer indicating how many instances
of the storage to create. If unspecified, COUNT defaults to 1.

Further details:

storage-add adds storage volumes to the unit.
storage-add takes the name of the storage volume (as defined in the
charm metadata), and optionally the number of storage instances to add.
By default, it will add a single storage instance of the name.
`

	var examples = `
    storage-add database-storage=1
`
	return jujucmd.Info(&cmd.Info{
		Name:     "storage-add",
		Args:     "<charm storage name>[=count] ...",
		Purpose:  "Add storage instances.",
		Doc:      doc,
		Examples: examples,
	})
}

func (s *StorageAddCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("storage add requires a storage directive")
	}

	constraintsMap, err := storage.ParseConstraintsMap(args, false)
	if err != nil {
		return errors.Trace(err)
	}

	s.all = make(map[string]params.StorageConstraints, len(constraintsMap))
	for k, v := range constraintsMap {
		cons := v
		if cons != (storage.Constraints{Count: cons.Count}) {
			return errors.Errorf("only count can be specified for %q", k)
		}
		s.all[k] = params.StorageConstraints{Count: &cons.Count}
	}
	return nil
}

func (s *StorageAddCommand) Run(ctx *cmd.Context) error {
	return s.ctx.AddUnitStorage(s.all)
}
