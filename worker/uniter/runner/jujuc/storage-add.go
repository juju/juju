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
and storage constraints, e.g. pool, count, size.

The acceptable format for storage constraints is a comma separated
sequence of: POOL, COUNT, and SIZE, where

    POOL identifies the storage pool. POOL can be a string
    starting with a letter, followed by zero or more digits
    or letters optionally separated by hyphens.

    COUNT is a positive integer indicating how many instances
    of the storage to create. If unspecified, and SIZE is
    specified, COUNT defaults to 1.

    SIZE describes the minimum size of the storage instances to
    create. SIZE is a floating point number and multiplier from
    the set (M, G, T, P, E, Z, Y), which are all treated as
    powers of 1024.

Storage constraints can be optionally ommitted.
Environment default values will be used for all ommitted constraint values.
There is no need to comma-separate ommitted constraints, e.g. 
    data=ebs,2,    <equivalent to>   data=ebs,2
    data=1         <equivalent to>   data
`

func (s *StorageAddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "storage-add",
		Args:    "<charm storage name>=<constraints> ...",
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
		s.all[k] = params.StorageConstraints{v.Pool, &v.Size, &v.Count}
	}
	return nil
}

func (s *StorageAddCommand) Run(ctx *cmd.Context) error {
	s.ctx.AddUnitStorage(s.all)
	return nil
}
