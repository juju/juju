// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.apiserver.common")

// AgentPasswordService defines the methods required to set an agent password
// hash.
type AgentPasswordService interface {
	// SetUnitPassword sets the password hash for the given unit.
	SetUnitPassword(context.Context, coreunit.Name, string) error

	// SetMachinePassword sets the password hash for the given machine.
	SetMachinePassword(context.Context, coremachine.Name, string) error

	// SetControllerNodePassword sets the password hash for the given
	// controller node.
	SetControllerNodePassword(context.Context, string, string) error

	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(ctx context.Context, machineName coremachine.Name) (bool, error)
}

// PasswordChanger implements a common SetPasswords method for use by
// various facades.
type PasswordChanger struct {
	agentPasswordService AgentPasswordService
	getCanChange         GetAuthFunc
}

// NewPasswordChanger returns a new PasswordChanger. The GetAuthFunc will be
// used on each invocation of SetPasswords to determine current permissions.
func NewPasswordChanger(agentPasswordService AgentPasswordService, getCanChange GetAuthFunc) *PasswordChanger {
	return &PasswordChanger{
		agentPasswordService: agentPasswordService,
		getCanChange:         getCanChange,
	}
}

// SetPasswords sets the given password for each supplied entity, if possible.
func (pc *PasswordChanger) SetPasswords(ctx context.Context, args params.EntityPasswords) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}
	canChange, err := pc.getCanChange(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, param := range args.Changes {
		tag, err := names.ParseTag(param.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canChange(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if err := pc.setPassword(ctx, tag, param.Password); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

func (pc *PasswordChanger) setPassword(ctx context.Context, tag names.Tag, password string) error {
	switch tag.Kind() {
	case names.UnitTagKind:
		unitTag := tag.(names.UnitTag)
		unitName := coreunit.Name(unitTag.Id())
		if err := pc.agentPasswordService.SetUnitPassword(ctx, unitName, password); errors.Is(err, applicationerrors.UnitNotFound) {
			return errors.NotFoundf("unit %q", tag.Id())
		} else if err != nil {
			return internalerrors.Errorf("setting password for %q: %w", tag, err)
		}
		return nil

	case names.MachineTagKind:
		machineTag := tag.(names.MachineTag)
		machineName := coremachine.Name(machineTag.Id())
		if err := pc.agentPasswordService.SetMachinePassword(ctx, machineName, password); errors.Is(err, machineerrors.MachineNotFound) {
			return errors.NotFoundf("machine %q", tag.Id())
		} else if err != nil {
			return internalerrors.Errorf("setting password for %q: %w", tag, err)
		}
		return nil

	case names.ControllerAgentTagKind:
		controllerTag := tag.(names.ControllerAgentTag)
		controllerName := controllerTag.Id()
		if err := pc.agentPasswordService.SetControllerNodePassword(ctx, controllerName, password); errors.Is(err, controllernodeerrors.NotFound) {
			return errors.NotFoundf("controller node %q", controllerName)
		} else if err != nil {
			return internalerrors.Errorf("setting password for %q: %w", tag, err)
		}
		return nil

	// TODO: Handle the following password setting:
	//  - model

	default:
		return errors.Errorf("unsupported tag %s", tag)
	}
}
