// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// EnsureDeadMachineService defines the methods that the facade assumes from the Machine
// service supporting EnsureDead methods
type EnsureDeadMachineService interface {
	// EnsureDeadMachine sets the provided machine's life status to Dead.
	// No error is returned if the provided machine doesn't exist, just nothing
	// gets updated.
	EnsureDeadMachine(ctx context.Context, machineName machine.Name) error
}

// DeadEnsurer implements a common EnsureDead method for use by
// various facades.
type DeadEnsurer struct {
	st             state.EntityFinder
	getCanModify   GetAuthFunc
	machineService EnsureDeadMachineService
}

// NewDeadEnsurer returns a new DeadEnsurer. The GetAuthFunc will be
// used on each invocation of EnsureDead to determine current
// permissions.
func NewDeadEnsurer(st state.EntityFinder, getCanModify GetAuthFunc, machineService EnsureDeadMachineService) *DeadEnsurer {
	return &DeadEnsurer{
		st:             st,
		getCanModify:   getCanModify,
		machineService: machineService,
	}
}

func (d *DeadEnsurer) ensureEntityDead(ctx context.Context, tag names.Tag) error {
	entity0, err := d.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.EnsureDeader)
	if !ok {
		return apiservererrors.NotSupportedError(tag, "ensuring death")
	}
	if err := entity.EnsureDead(); err != nil {
		return errors.Trace(err)
	}
	// Double write the Dead life status on dqlite if the entity is a machine.
	if tag.Kind() == names.MachineTagKind {
		if err := d.machineService.EnsureDeadMachine(ctx, machine.Name(tag.Id())); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// EnsureDead calls EnsureDead on each given entity from state. It
// will fail if the entity is not present. If it's Alive, nothing will
// happen (see state/EnsureDead() for units or machines).
func (d *DeadEnsurer) EnsureDead(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := d.getCanModify(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}

		err = apiservererrors.ErrPerm
		if canModify(tag) {
			err = d.ensureEntityDead(ctx, tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
