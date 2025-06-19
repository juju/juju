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
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver.common")

// AgentPasswordService defines the methods required to set an agent password
// hash.
type AgentPasswordService interface {
	// SetUnitPassword sets the password hash for the given unit.
	SetUnitPassword(context.Context, coreunit.Name, string) error

	// SetMachinePassword sets the password hash for the given machine.
	SetMachinePassword(context.Context, coremachine.Name, string) error
}

// ControllerNodeService defines the methods required to set a controller node's
// password hash.
type ControllerNodeService interface {
	// SetPassword sets the password for the given machine. If the controller node
	// does not exist, an error satisfying [controllernodeerrors.NotFound] is returned.
	SetPassword(ctx context.Context, nodeID string, password string) error
}

// PasswordChanger implements a common SetPasswords method for use by
// various facades.
type PasswordChanger struct {
	agentPasswordService  AgentPasswordService
	controllerNodeService ControllerNodeService
	st                    state.EntityFinder
	getCanChange          GetAuthFunc
}

// NewPasswordChanger returns a new PasswordChanger. The GetAuthFunc will be
// used on each invocation of SetPasswords to determine current permissions.
func NewPasswordChanger(
	agentPasswordService AgentPasswordService,
	controllerNodeService ControllerNodeService,
	st state.EntityFinder, getCanChange GetAuthFunc) *PasswordChanger {
	return &PasswordChanger{
		agentPasswordService:  agentPasswordService,
		controllerNodeService: controllerNodeService,
		st:                    st,
		getCanChange:          getCanChange,
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

		// TODO (stickupkid): This should be removed once we delete mongo.
		return pc.legacyMachineSetPassword(tag, password)

	case names.ControllerAgentTagKind:
		if pc.controllerNodeService == nil {
			return internalerrors.New("changing controller node password")
		}
		controllerAgentTag := tag.(names.ControllerAgentTag)
		if err := pc.controllerNodeService.SetPassword(ctx, controllerAgentTag.Id(), password); errors.Is(err, controllernodeerrors.NotFound) {
			return errors.NotFoundf("controller node %q", tag.Id())
		} else if err != nil {
			return internalerrors.Errorf("setting password for %q: %w", tag, err)
		}
		return nil

	// TODO: Handle the following password setting:
	//  - model

	default:
		return pc.legacySetPassword(tag, password)
	}
}

func (pc *PasswordChanger) legacyMachineSetPassword(tag names.Tag, password string) error {
	// This is being removed, this is to ensure we just set up the mongo
	// password. If the state is nil, just ignore the request.
	if pc.st == nil {
		return nil
	}

	entity0, err := pc.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(*state.Machine)
	if !ok {
		return apiservererrors.NotSupportedError(tag, "authentication")
	}
	if !entity.IsManager() {
		return nil
	}

	return pc.setMongoPassword(entity0, password)
}

// setMongoPassword applies to controller machines.
func (pc *PasswordChanger) setMongoPassword(entity state.Entity, password string) error {
	type mongoPassworder interface {
		SetMongoPassword(password string) error
	}
	// We set the mongo password first on the grounds that
	// if it fails, the agent in question should still be able
	// to authenticate to another API server and ask it to change
	// its password.
	if entity0, ok := entity.(mongoPassworder); ok {
		if err := entity0.SetMongoPassword(password); err != nil {
			return err
		}
		logger.Infof(context.TODO(), "setting mongo password for %q", entity.Tag())
		return nil
	}
	// TODO(dfc) fix
	return apiservererrors.NotSupportedError(entity.Tag(), "mongo access")
}

func (pc *PasswordChanger) legacySetPassword(tag names.Tag, password string) error {
	// This is being removed, this is to ensure we just set up the mongo
	// password. If the state is nil, just ignore the request.
	if pc.st == nil {
		return nil
	}

	var err error
	entity0, err := pc.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.Authenticator)
	if !ok {
		return apiservererrors.NotSupportedError(tag, "authentication")
	}

	err = entity.SetPassword(password)
	logger.Infof(context.TODO(), "setting password for %q", tag)

	return err
}
