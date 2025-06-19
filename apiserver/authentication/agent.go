// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/state"
)

// AgentPasswordService defines the methods required to set an agent password
// hash.
type AgentPasswordService interface {
	// MatchesUnitPasswordHash checks if the password is valid or not.
	MatchesUnitPasswordHash(context.Context, unit.Name, string) (bool, error)

	// MatchesMachinePasswordHashWithNonce checks if the password with a nonce
	// is valid or not.
	MatchesMachinePasswordHashWithNonce(context.Context, machine.Name, string, string) (bool, error)

	// IsMachineController checks if the machine is a controller.
	IsMachineController(context.Context, machine.Name) (bool, error)
}

// AgentAuthenticatorGetter is a factory for creating authenticators, which
// can create authenticators for a given state.
type AgentAuthenticatorGetter struct {
	agentPasswordService AgentPasswordService
	legacyState          *state.State
	logger               corelogger.Logger
}

// NewAgentAuthenticatorGetter returns a new agent authenticator factory, for
// a known state.
func NewAgentAuthenticatorGetter(agentPasswordService AgentPasswordService, legacy *state.State, logger corelogger.Logger) AgentAuthenticatorGetter {
	return AgentAuthenticatorGetter{
		agentPasswordService: agentPasswordService,
		legacyState:          legacy,
		logger:               logger,
	}
}

// Authenticator returns an authenticator using the factory's controller model.
func (f AgentAuthenticatorGetter) Authenticator() EntityAuthenticator {
	return agentAuthenticator{
		agentPasswordService: f.agentPasswordService,
		state:                f.legacyState,
		logger:               f.logger,
	}
}

// AuthenticatorForModel returns an authenticator for the given model.
func (f AgentAuthenticatorGetter) AuthenticatorForModel(agentPasswordService AgentPasswordService) EntityAuthenticator {
	return agentAuthenticator{
		agentPasswordService: agentPasswordService,
		logger:               f.logger,
	}
}

type agentAuthenticator struct {
	agentPasswordService AgentPasswordService
	state                *state.State
	logger               corelogger.Logger
}

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// Authenticate authenticates the provided entity.
// It takes an entityfinder and the tag used to find the entity that requires authentication.
func (a agentAuthenticator) Authenticate(ctx context.Context, authParams AuthParams) (state.Entity, error) {
	switch authParams.AuthTag.Kind() {
	case names.UserTagKind:
		return nil, errors.Trace(fmt.Errorf("user authentication: %w", apiservererrors.ErrBadRequest))

	case names.UnitTagKind:
		return a.authenticateUnit(ctx, authParams.AuthTag.(names.UnitTag), authParams.Credentials)

	case names.MachineTagKind:
		return a.authenticateMachine(ctx, authParams.AuthTag.(names.MachineTag), authParams.Credentials, authParams.Nonce)

	default:
		entity, err := a.fallbackAuth(ctx, authParams)
		return entity, err
	}
}

func (a *agentAuthenticator) authenticateUnit(ctx context.Context, tag names.UnitTag, credentials string) (state.Entity, error) {
	unitName := unit.Name(tag.Id())

	// Check if the password is correct.
	// - If the password is empty, then we consider that a bad request
	//   (incorrect payload).
	// - If the password is invalid, then we consider that unauthorized.
	// - If the unit is not found, then we consider that unauthorized. Prevent
	//   the knowing about which unit the password didn't match (rainbow attack)
	// - If the password isn't valid for the unit, then we consider that
	//   unauthorized.
	// - Any other error, is considered an internal server error.

	valid, err := a.agentPasswordService.MatchesUnitPasswordHash(ctx, unitName, credentials)
	if errors.Is(err, agentpassworderrors.EmptyPassword) {
		return nil, errors.Trace(fmt.Errorf("unit authentication: %w", apiservererrors.ErrBadRequest))
	} else if errors.Is(err, agentpassworderrors.InvalidPassword) || errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
	} else if err != nil {
		return nil, errors.Trace(err)
	} else if !valid {
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
	}

	return TagToEntity(tag), nil
}

func (a *agentAuthenticator) authenticateMachine(ctx context.Context, tag names.MachineTag, credentials, nonce string) (state.Entity, error) {
	machineName := machine.Name(tag.Id())

	// Check if the password is correct.
	// - If the password or nonce is empty, then we consider that a bad request
	//   (incorrect payload).
	// - If the password is invalid, then we consider that unauthorized.
	// - If the machine is not found, then we consider that unauthorized.
	//   Prevent the knowing about which machine the password didn't match
	//   (rainbow attack).
	// - If the password isn't valid for the machine, then we consider that
	//   unauthorized.
	// - If the machine is not provisioned, then we consider that the machine
	//   is not provisioned (the password must match first before undertaking
	//   the provisioning).
	// - Any other error, is considered an internal server error.

	valid, err := a.agentPasswordService.MatchesMachinePasswordHashWithNonce(ctx, machineName, credentials, nonce)
	if errors.Is(err, agentpassworderrors.EmptyPassword) || errors.Is(err, agentpassworderrors.EmptyNonce) {
		return nil, errors.Trace(fmt.Errorf("machine authentication: %w", apiservererrors.ErrBadRequest))
	} else if errors.Is(err, agentpassworderrors.InvalidPassword) || errors.Is(err, applicationerrors.MachineNotFound) {
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
	} else if errors.Is(err, machineerrors.NotProvisioned) {
		return nil, errors.NotProvisionedf("machine %v", tag.Id())
	} else if err != nil {
		return nil, errors.Trace(err)
	} else if !valid {
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
	}

	return TagToEntity(tag), nil
}

func (a *agentAuthenticator) fallbackAuth(ctx context.Context, authParams AuthParams) (state.Entity, error) {
	entity, err := a.state.FindEntity(authParams.AuthTag)
	if errors.Is(err, errors.NotFound) {
		logger.Debugf(ctx, "cannot authenticate unknown entity: %v", authParams.AuthTag)
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	authenticator, ok := entity.(taggedAuthenticator)
	if !ok {
		return nil, errors.Trace(fmt.Errorf("Authenticate fallback: %w", apiservererrors.ErrBadRequest))
	}
	if !authenticator.PasswordValid(authParams.Credentials) {
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
	}

	return entity, nil
}
