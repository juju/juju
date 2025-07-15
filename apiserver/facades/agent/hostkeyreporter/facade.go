// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"context"
	"errors"

	"github.com/juju/names/v6"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
)

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns an errors.MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)
	// SetSSHHostKeys sets the SSH host keys for the given machine UUID.
	SetSSHHostKeys(ctx context.Context, mUUID machine.UUID, keys []string) error
}

// Facade implements the API required by the hostkeyreporter worker.
type Facade struct {
	machineService MachineService
	getCanModify   common.GetAuthFunc
}

// New returns a new API facade for the hostkeyreporter worker.
func New(machineService MachineService, authorizer facade.Authorizer) (*Facade, error) {
	return &Facade{
		machineService: machineService,
		getCanModify: func(context.Context) (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
		},
	}, nil
}

// ReportKeys sets the SSH host keys for one or more entities.
func (facade *Facade) ReportKeys(ctx context.Context, args params.SSHHostKeySet) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.EntityKeys)),
	}

	canModify, err := facade.getCanModify(ctx)
	if err != nil {
		return results, err
	}

	for i, arg := range args.EntityKeys {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canModify(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineUUID, err := facade.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ServerError(jujuerrors.NotFoundf("machine %q", tag))
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = facade.machineService.SetSSHHostKeys(ctx, machineUUID, arg.PublicKeys)
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ServerError(jujuerrors.NotFoundf("machine %q", tag))
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}
