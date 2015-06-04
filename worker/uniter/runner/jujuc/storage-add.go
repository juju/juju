// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
)

// StorageAddCommand implements the status-set command.
type StorageAddCommand struct {
	cmd.CommandBase
	ctx Context
	all map[string]params.StorageConstraints
}

// NewStorageAddCommand makes a jujuc storage-add command.
func NewStorageAddCommand(ctx Context) cmd.Command {
	return &StorageAddCommand{ctx: ctx}
}

var StorageAddDoc = `
Storage add adds storage instances to unit using provided storage directives.
A storage directive consists of a storage name as per charm specification
and optional storage COUNT.

COUNT is a positive integer indicating how many instances
of the storage to create. If unspecified, COUNT defaults to 1.
`

func (s *StorageAddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "storage-add",
		Args:    "<charm storage name>[=count] ...",
		Purpose: "add storage instances",
		Doc:     StorageAddDoc,
	}
}

func (s *StorageAddCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("storage add requires a storage directive")
	}

	cons, err := storage.ParseConstraintsMap(args, false)
	if err != nil {
		return errors.Trace(err)
	}

	s.all = make(map[string]params.StorageConstraints, len(cons))
	for k, v := range cons {
		if v != (storage.Constraints{Count: v.Count}) {
			return errors.Errorf("only count can be specified for %q", k)
		}
		s.all[k] = params.StorageConstraints{Count: &v.Count}
	}
	return nil
}

func (s *StorageAddCommand) Run(ctx *cmd.Context) error {
	s.ctx.AddUnitStorage(s.all)
	return nil
}
