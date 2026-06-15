// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strconv"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/errors"
	pkissh "github.com/juju/juju/internal/pki/ssh"
)

// Service provides controller and model scoped SSH host key workflows.
type Service struct {
	controllerSt     ControllerState
	modelStateGetter ModelStateGetter
}

// NewService returns a new SSH service.
func NewService(controllerSt ControllerState, modelStateGetter ModelStateGetter) *Service {
	return &Service{
		controllerSt:     controllerSt,
		modelStateGetter: modelStateGetter,
	}
}

// SSHServerHostKey returns the controller jump host key.
func (s *Service) SSHServerHostKey(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if s.controllerSt == nil {
		return "", errors.Errorf("missing controller SSH state")
	}

	key, found, err := s.controllerSt.GetSSHServerHostKey(ctx)
	if err != nil {
		return "", errors.Errorf("getting controller SSH server host key: %w", err)
	}
	if !found {
		return "", errors.Errorf("controller SSH server host key not found")
	}
	return key, nil
}

// VirtualHostKey resolves the terminating SSH host key for a virtual hostname.
func (s *Service) VirtualHostKey(ctx context.Context, info virtualhostname.Info) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelUUID := coremodel.UUID(info.ModelUUID())
	if err := modelUUID.Validate(); err != nil {
		return "", errors.Errorf("validating model UUID %q: %w", modelUUID, err)
	}

	switch info.Target() {
	case virtualhostname.MachineTarget:
		machineNumber, ok := info.Machine()
		if !ok {
			return "", errors.Errorf("missing machine target in virtual hostname")
		}
		machineName := coremachine.Name(strconv.Itoa(machineNumber))
		return s.MachineVirtualHostKey(ctx, modelUUID, machineName)
	case virtualhostname.UnitTarget, virtualhostname.ContainerTarget:
		unitName, ok := info.Unit()
		if !ok {
			return "", errors.Errorf("missing unit target in virtual hostname")
		}
		parsedUnitName, err := coreunit.NewName(unitName)
		if err != nil {
			return "", errors.Errorf("validating unit name %q: %w", unitName, err)
		}
		return s.UnitVirtualHostKey(ctx, modelUUID, parsedUnitName)
	default:
		return "", errors.Errorf("virtual hostname target %d %w", info.Target(), coreerrors.NotSupported)
	}
}

// MachineVirtualHostKey returns the machine terminating host key, generating
// and persisting it if it is missing.
func (s *Service) MachineVirtualHostKey(ctx context.Context, modelUUID coremodel.UUID, machineName coremachine.Name) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return "", errors.Errorf("validating model UUID %q: %w", modelUUID, err)
	}
	if err := machineName.Validate(); err != nil {
		return "", errors.Errorf("validating machine name %q: %w", machineName, err)
	}

	state, err := s.modelState(modelUUID)
	if err != nil {
		return "", err
	}
	return s.ensureMachineVirtualHostKey(ctx, state, machineName.String())
}

// UnitVirtualHostKey returns the terminating host key for a unit target.
// IAAS units share the host key of their backing machine, while CAAS units use
// a unit-scoped virtual host key.
func (s *Service) UnitVirtualHostKey(ctx context.Context, modelUUID coremodel.UUID, unitName coreunit.Name) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return "", errors.Errorf("validating model UUID %q: %w", modelUUID, err)
	}
	if err := unitName.Validate(); err != nil {
		return "", errors.Errorf("validating unit name %q: %w", unitName, err)
	}

	state, err := s.modelState(modelUUID)
	if err != nil {
		return "", err
	}

	machineName, machineBacked, err := state.GetMachineNameForUnit(ctx, unitName.String())
	if err != nil {
		return "", errors.Errorf("resolving backing machine for unit %q in model %q: %w", unitName, modelUUID, err)
	}
	if machineBacked {
		return s.ensureMachineVirtualHostKey(ctx, state, machineName)
	}
	return s.ensureUnitVirtualHostKey(ctx, state, unitName.String())
}

func (s *Service) modelState(modelUUID coremodel.UUID) (ModelState, error) {
	if s.modelStateGetter == nil {
		return nil, errors.Errorf("missing model SSH state getter")
	}
	state := s.modelStateGetter.GetModelState(modelUUID)
	if state == nil {
		return nil, errors.Errorf("missing model SSH state for model %q", modelUUID)
	}
	return state, nil
}

func (s *Service) ensureMachineVirtualHostKey(ctx context.Context, state ModelState, machineName string) (string, error) {
	key, found, err := state.GetMachineVirtualHostKeyByMachineName(ctx, machineName)
	if err != nil {
		return "", errors.Errorf("getting machine virtual SSH host key for %q: %w", machineName, err)
	}
	if found {
		return key, nil
	}

	key, err = generateHostKey()
	if err != nil {
		return "", errors.Errorf("generating machine virtual SSH host key for %q: %w", machineName, err)
	}
	if err := state.SetMachineVirtualHostKeyByMachineName(ctx, machineName, key); err != nil {
		return "", errors.Errorf("persisting machine virtual SSH host key for %q: %w", machineName, err)
	}
	return key, nil
}

func (s *Service) ensureUnitVirtualHostKey(ctx context.Context, state ModelState, unitName string) (string, error) {
	key, found, err := state.GetUnitVirtualHostKeyByUnitName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting unit virtual SSH host key for %q: %w", unitName, err)
	}
	if found {
		return key, nil
	}

	key, err = generateHostKey()
	if err != nil {
		return "", errors.Errorf("generating unit virtual SSH host key for %q: %w", unitName, err)
	}
	if err := state.SetUnitVirtualHostKeyByUnitName(ctx, unitName, key); err != nil {
		return "", errors.Errorf("persisting unit virtual SSH host key for %q: %w", unitName, err)
	}
	return key, nil
}

func generateHostKey() (string, error) {
	key, err := pkissh.NewMarshalledED25519()
	if err != nil {
		return "", errors.Capture(err)
	}
	return string(key), nil
}
